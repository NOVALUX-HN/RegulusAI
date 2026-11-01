package httpapi

// dashboardHTML is the always-on visual (D2): a professional panel with
// cards per area (Guardado/Evidencia/Memoria/Aprendizaje/Coste), plain
// language explanations, paginated tables over the read-only /v1/*
// collections and dependency-free mini charts (CSS bars). No external JS,
// no writes, stdlib fetch only. All dynamic text is set through
// textContent (never innerHTML with data).
const dashboardHTML = `<!doctype html>
<html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Regulus — Qué hay guardado en tu proyecto</title>
<style>
:root{--bg:#f4f6fb;--card:#ffffff;--ink:#1c2333;--muted:#5b6577;--line:#e2e8f2;--accent:#2456d6;--good:#157f3d;--warn:#a86a00;--bad:#b3261e}
*{box-sizing:border-box}
body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;margin:0;background:var(--bg);color:var(--ink)}
header.top{background:#16213a;color:#fff;padding:28px 24px}
header.top .wrap,main{max-width:1100px;margin:0 auto}
header.top h1{margin:0 0 8px;font-size:28px}
header.top p{margin:4px 0;color:#c6d2ea}
#status,#health{margin-top:12px;display:inline-block;background:#0e1830;border:1px solid #33456b;border-radius:8px;padding:8px 14px;font-size:14px;margin-right:8px}
main{padding:24px}
section{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:20px;margin:0 0 20px}
h2{margin:0 0 4px;font-size:20px}
p.plain{color:var(--muted);margin:6px 0 14px;font-size:14px}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}
.card{border:1px solid var(--line);border-radius:10px;padding:14px;background:#fbfcff}
.card h3{margin:0 0 4px;font-size:15px}
.card .num{font-size:32px;font-weight:700;color:var(--accent)}
.card p{margin:6px 0 0;font-size:13px;color:var(--muted)}
.bar-row{display:grid;grid-template-columns:180px 1fr 60px;gap:8px;align-items:center;margin:6px 0;font-size:13px}
.bar-track{background:#e9edf5;border-radius:6px;height:14px;overflow:hidden}
.bar-fill{height:100%;background:var(--accent);border-radius:6px}
.bar-row .v{text-align:right;font-variant-numeric:tabular-nums}
table{width:100%;border-collapse:collapse;font-size:14px}
th,td{text-align:left;padding:8px;border-bottom:1px solid var(--line);vertical-align:top}
th{background:#f1f4fa;font-size:12px;text-transform:uppercase;letter-spacing:.04em;color:var(--muted)}
.pager{display:flex;gap:8px;align-items:center;margin-top:10px;font-size:13px;color:var(--muted)}
button{border:1px solid var(--line);background:#fff;border-radius:8px;padding:6px 12px;cursor:pointer;font-size:14px}
button:disabled{opacity:.45;cursor:default}
button.primary{background:var(--accent);color:#fff;border-color:var(--accent)}
.controls{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:10px}
input,select{border:1px solid var(--line);border-radius:8px;padding:6px 10px;font-size:14px;background:#fff}
.ok{color:var(--good);font-weight:600}.fail{color:var(--bad);font-weight:600}
.err{color:var(--bad);font-size:13px}
footer{color:var(--muted);font-size:13px;text-align:center;padding:0 0 28px}
details{margin-top:8px;font-size:13px}
pre{background:#0e1830;color:#d7e1f5;padding:12px;border-radius:8px;overflow:auto;font-size:12px}
</style>
</head><body>
<header class="top"><div class="wrap">
<h1>Regulus — Qué hay guardado en tu proyecto</h1>
<p>Todo lo que Regulus guarda en tu ordenador, explicado en palabras sencillas. Esta página solo mira: nunca cambia ni borra nada.</p>
<div id="status">Estado del proyecto: cargando…</div>
<div id="health">Salud del sistema: cargando…</div>
<details><summary>Ver estado completo del proyecto</summary><pre id="status-full">cargando…</pre></details>
</div></header>
<main>
<section id="cards"><h2>Resumen por áreas</h2>
<p class="plain">Cada tarjeta cuenta cuántas cosas hay guardadas en una parte del proyecto y qué significa.</p>
<div class="grid" id="cards-grid"></div>
<p class="err" id="cards-error"></p></section>
<section id="graphs"><h2>De un vistazo</h2>
<p class="plain">Mini gráficos con los repartos más útiles: qué tipos de recuerdos hay, lecciones confirmadas frente a pendientes, en qué se usó el presupuesto y cómo salieron las comprobaciones.</p>
<div id="graphs"></div>
<p class="err" id="graphs-error"></p></section>
<section id="sec-evidencia"><h2>Evidencia — comprobantes de verificación</h2>
<p class="plain">Cada fila es una vez que Regulus comprobó que el trabajo estaba bien. Es el ticket de la prueba: qué tarea, qué prueba y si salió bien.</p>
<table><thead><tr><th>Tarea</th><th>Prueba</th><th>Resultado</th><th>Fecha</th></tr></thead><tbody id="tb-receipts"></tbody></table>
<div class="pager"><button id="prev-receipts">Anterior</button><span id="info-receipts"></span><button id="next-receipts">Siguiente</button></div>
<p class="err" id="err-receipts"></p></section>
<section id="sec-memoria"><h2>Memoria — notas y decisiones</h2>
<p class="plain">Lo que Regulus anotó para no olvidarlo: notas de trabajo, decisiones y descubrimientos. Usa el buscador para encontrar algo concreto.</p>
<div class="controls"><input id="q-obs" type="search" placeholder="Buscar en memoria…" aria-label="Buscar en memoria"><select id="type-obs" aria-label="Tipo de recuerdo"><option value="">Todos los tipos</option><option value="session-note">Notas</option><option value="decision">Decisiones</option><option value="discovery">Descubrimientos</option><option value="lesson">Lecciones</option><option value="preference">Preferencias</option></select><button id="go-obs" class="primary">Buscar</button></div>
<table><thead><tr><th>Título</th><th>Tipo</th><th>Fecha</th></tr></thead><tbody id="tb-obs"></tbody></table>
<div class="pager"><button id="prev-obs">Anterior</button><span id="info-obs"></span><button id="next-obs">Siguiente</button></div>
<p class="err" id="err-obs"></p></section>
<section id="sec-aprendizaje"><h2>Aprendizaje — lecciones</h2>
<p class="plain">Lo que Regulus ya aprendió (confirmado) y lo que cree haber aprendido pero aún está por confirmar (candidato).</p>
<div class="controls"><select id="status-lessons" aria-label="Estado de la lección"><option value="all">Confirmadas + por confirmar</option><option value="admitted">Solo confirmadas</option><option value="candidate">Solo por confirmar</option></select><button id="go-lessons" class="primary">Filtrar</button></div>
<table><thead><tr><th>Tarea</th><th>Estado</th><th>Lección</th></tr></thead><tbody id="tb-lessons"></tbody></table>
<div class="pager"><button id="prev-lessons">Anterior</button><span id="info-lessons"></span><button id="next-lessons">Siguiente</button></div>
<p class="err" id="err-lessons"></p></section>
<section id="sec-sesiones"><h2>Sesiones — el trabajo paso a paso</h2>
<p class="plain">Cada sesión es una racha de trabajo seguida. Pulsa «Ver pasos» para leer qué hizo Regulus en cada paso, en orden.</p>
<table><thead><tr><th>Sesión</th><th>Número de pasos</th><th></th></tr></thead><tbody id="tb-traces"></tbody></table>
<div class="pager"><button id="prev-traces">Anterior</button><span id="info-traces"></span><button id="next-traces">Siguiente</button></div>
<h3 id="trace-title">Pasos de la sesión</h3>
<table><thead><tr><th>Paso</th><th>Herramienta</th><th>Qué hizo</th></tr></thead><tbody id="tb-entries"></tbody></table>
<div class="pager"><button id="prev-entries">Anterior</button><span id="info-entries"></span><button id="next-entries">Siguiente</button></div>
<p class="err" id="err-traces"></p></section>
<section id="sec-coste"><h2>Coste — presupuesto de mejora</h2>
<p class="plain">Cada fila es un movimiento del presupuesto que Regulus usa para mejorar solo: qué día, de qué tipo, en qué tarea y cómo terminó.</p>
<table><thead><tr><th>Fecha</th><th>Tipo</th><th>Tarea</th><th>Resultado</th></tr></thead><tbody id="tb-budget"></tbody></table>
<div class="pager"><button id="prev-budget">Anterior</button><span id="info-budget"></span><button id="next-budget">Siguiente</button></div>
<p class="err" id="err-budget"></p></section>
</main>
<footer>Se actualiza solo cada 30 segundos. <button id="refresh">Actualizar ahora</button></footer>
<script>
var PAGE = 10;
var state = {receipts:0, obs:0, lessons:0, traces:0, entries:0, budget:0, session:''};
function fetchData(path){
  return fetch(path).then(function(r){return r.json();}).then(function(env){
    if(!env || env.ok !== true || !env.data){throw new Error('respuesta inesperada');}
    return env.data;
  });
}
function setText(id, s){document.getElementById(id).textContent = s;}
function setError(id, e){setText(id, e ? 'No se pudo cargar: ' + (e.message || e) : '');}
function td(s){var c = document.createElement('td');c.textContent = s;return c;}
function pageInfo(infoId, prevId, nextId, total, limit, offset){
  var lo = total === 0 ? 0 : offset + 1;
  var hi = Math.min(offset + limit, total);
  setText(infoId, 'Mostrando ' + lo + '–' + hi + ' de ' + total);
  document.getElementById(prevId).disabled = offset <= 0;
  document.getElementById(nextId).disabled = offset + limit >= total;
}
function card(title, value, what){
  var a = document.createElement('article');a.className = 'card';
  var h = document.createElement('h3');h.textContent = title;a.appendChild(h);
  var n = document.createElement('div');n.className = 'num';n.textContent = String(value);a.appendChild(n);
  var p = document.createElement('p');p.textContent = what;a.appendChild(p);
  return a;
}
function barRow(box, label, value, max){
  var row = document.createElement('div');row.className = 'bar-row';
  var l = document.createElement('span');l.textContent = label;row.appendChild(l);
  var track = document.createElement('div');track.className = 'bar-track';
  var fill = document.createElement('div');fill.className = 'bar-fill';
  fill.style.width = (max > 0 ? Math.round(100 * value / max) : 0) + '%';
  track.appendChild(fill);row.appendChild(track);
  var v = document.createElement('span');v.className = 'v';v.textContent = String(value);row.appendChild(v);
  box.appendChild(row);
}
function barGroup(box, title, pairs){
  var h = document.createElement('h3');h.textContent = title;box.appendChild(h);
  var max = 0;
  pairs.forEach(function(p){if(p[1] > max){max = p[1];}});
  if(pairs.length === 0){
    var e = document.createElement('p');e.className = 'plain';e.textContent = 'Sin datos todavía.';box.appendChild(e);return;
  }
  pairs.forEach(function(p){barRow(box, p[0], p[1], max);});
}
function loadHeader(){
  Promise.all([fetchData('/v1/status'), fetchData('/v1/version'), fetchData('/v1/metrics')]).then(function(r){
    var st = r[0], ver = r[1], me = r[2];
    var ok = st.exit === 0;
    setText('status', 'Estado del proyecto: ' + (ok ? 'bien' : 'revisar (código ' + st.exit + ')') + ' · ' + (ver.version || '').trim());
    setText('status-full', (st.stdout || '') + (st.stderr ? '\n' + st.stderr : ''));
    var m = me.metrics || {};
    var evo = 'Mejoras usadas hoy: ' + me.evolution_used + ' de ' + me.evolution_cap;
    var pass = m.verification_pass_rate;
    setText('health', 'Salud del sistema: ' + m.tasks + ' tareas, ' + m.receipts_pass + ' de ' + m.receipts_total + ' comprobaciones bien' + (pass ? ' (' + Math.round(pass * 100) + '%)' : '') + ' · ' + evo);
  }).catch(function(e){
    setText('status', 'Estado del proyecto: no disponible');
    setText('health', 'Salud del sistema: no disponible');
    setText('status-full', 'No se pudo cargar: ' + (e.message || e));
  });
}
function loadCards(){
  Promise.all([
    fetchData('/v1/almanac/summary'),
    fetchData('/v1/receipts?limit=1'),
    fetchData('/v1/observations?limit=1'),
    fetchData('/v1/lessons?limit=1'),
    fetchData('/v1/budget?limit=1')
  ]).then(function(r){
    var box = document.getElementById('cards-grid');
    while(box.firstChild){box.removeChild(box.firstChild);}
    box.appendChild(card('Guardado', r[0].total, 'Cosas guardadas en total: notas, decisiones y recuerdos que Regulus conserva de tu proyecto.'));
    box.appendChild(card('Evidencia', r[1].total, 'Comprobantes de verificación: veces que se comprobó que el trabajo estaba bien.'));
    box.appendChild(card('Memoria', r[2].total, 'Recuerdos anotados: notas y decisiones para no olvidar lo importante.'));
    var lt = (r[3].total !== undefined) ? r[3].total : 0;
    box.appendChild(card('Aprendizaje', lt, 'Lecciones: lo ya confirmado más lo que está por confirmar.'));
    box.appendChild(card('Coste', r[4].total, 'Movimientos del presupuesto de mejora: cuánto trabajo de mejora se ha usado.'));
    setError('cards-error', null);
  }).catch(function(e){setError('cards-error', e);});
}
function loadGraphs(){
  Promise.all([
    fetchData('/v1/almanac/summary'),
    fetchData('/v1/lessons?limit=1'),
    fetchData('/v1/budget?limit=200'),
    fetchData('/v1/receipts?limit=200')
  ]).then(function(r){
    var box = document.getElementById('graphs');
    while(box.firstChild){box.removeChild(box.firstChild);}
    var byType = [];
    var bt = r[0].by_type || {};
    Object.keys(bt).sort().forEach(function(k){byType.push([k, bt[k]]);});
    barGroup(box, 'Recuerdos por tipo', byType);
    var adm = r[0].lessons_admitted || 0, cand = r[0].candidates_pending || 0;
    void r[1];
    barGroup(box, 'Lecciones confirmadas frente a por confirmar', [['Confirmadas', adm], ['Por confirmar', cand]]);
    var byKind = {};
    (r[2].items || []).forEach(function(e){var k = e.kind || '?';byKind[k] = (byKind[k] || 0) + 1;});
    var kinds = Object.keys(byKind).sort().map(function(k){return [k, byKind[k]];});
    barGroup(box, 'Presupuesto usado por tipo', kinds);
    var byVerdict = {};
    (r[3].items || []).forEach(function(it){
      var v = (it.receipt && it.receipt.verdict) || '?';
      byVerdict[v] = (byVerdict[v] || 0) + 1;
    });
    var verdicts = Object.keys(byVerdict).sort().map(function(k){return [k, byVerdict[k]];});
    barGroup(box, 'Comprobaciones por resultado', verdicts);
    setError('graphs-error', null);
  }).catch(function(e){setError('graphs-error', e);});
}
function loadReceipts(){
  fetchData('/v1/receipts?limit=' + PAGE + '&offset=' + state.receipts).then(function(d){
    var tb = document.getElementById('tb-receipts');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    (d.items || []).forEach(function(it){
      var tr = document.createElement('tr');
      var rc = it.receipt || {};
      tr.appendChild(td(it.task || rc.task || ''));
      tr.appendChild(td(rc.gate || ''));
      var v = document.createElement('td');
      var s = document.createElement('span');
      s.textContent = rc.verdict || '';
      s.className = rc.verdict === 'pass' ? 'ok' : 'fail';
      v.appendChild(s);tr.appendChild(v);
      var started = rc.started_at || rc.startedAt || '';
      tr.appendChild(td(String(started).slice(0, 19).replace('T', ' ')));
      tb.appendChild(tr);
    });
    pageInfo('info-receipts', 'prev-receipts', 'next-receipts', d.total, d.limit, d.offset);
    setError('err-receipts', null);
  }).catch(function(e){setError('err-receipts', e);});
}
function loadObs(){
  var q = document.getElementById('q-obs').value;
  var t = document.getElementById('type-obs').value;
  var url = '/v1/observations?limit=' + PAGE + '&offset=' + state.obs;
  if(q){url += '&q=' + encodeURIComponent(q);}
  if(t){url += '&type=' + encodeURIComponent(t);}
  fetchData(url).then(function(d){
    var tb = document.getElementById('tb-obs');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    (d.items || []).forEach(function(o){
      var tr = document.createElement('tr');
      tr.appendChild(td(o.title || '(sin título)'));
      tr.appendChild(td(o.type || ''));
      tr.appendChild(td(String(o.saved_at || o.savedAt || '').slice(0, 10)));
      tb.appendChild(tr);
    });
    pageInfo('info-obs', 'prev-obs', 'next-obs', d.total, d.limit, d.offset);
    setError('err-obs', null);
  }).catch(function(e){setError('err-obs', e);});
}
function loadLessons(){
  var s = document.getElementById('status-lessons').value;
  fetchData('/v1/lessons?status=' + encodeURIComponent(s) + '&limit=' + PAGE + '&offset=' + state.lessons).then(function(d){
    var tb = document.getElementById('tb-lessons');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    (d.items || []).forEach(function(l){
      var tr = document.createElement('tr');
      tr.appendChild(td(l.task || ''));
      var st = document.createElement('td');
      var tag = document.createElement('span');
      tag.textContent = l.status === 'candidate' ? 'Por confirmar' : 'Confirmada';
      tag.className = l.status === 'candidate' ? 'fail' : 'ok';
      st.appendChild(tag);tr.appendChild(st);
      var txt = l.lesson || l.cue || '';
      tr.appendChild(td(txt.length > 140 ? txt.slice(0, 140) + '…' : txt));
      tb.appendChild(tr);
    });
    pageInfo('info-lessons', 'prev-lessons', 'next-lessons', d.total, d.limit, d.offset);
    setError('err-lessons', null);
  }).catch(function(e){setError('err-lessons', e);});
}
function loadTraces(){
  fetchData('/v1/traces?limit=' + PAGE + '&offset=' + state.traces).then(function(d){
    var tb = document.getElementById('tb-traces');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    (d.items || []).forEach(function(s){
      var tr = document.createElement('tr');
      tr.appendChild(td(s.session || ''));
      tr.appendChild(td(String(s.entries)));
      var act = document.createElement('td');
      var b = document.createElement('button');
      b.textContent = 'Ver pasos';
      (function(name){
        b.addEventListener('click', function(){
          state.session = name;state.entries = 0;loadEntries();
        });
      })(s.session);
      act.appendChild(b);tr.appendChild(act);
      tb.appendChild(tr);
    });
    pageInfo('info-traces', 'prev-traces', 'next-traces', d.total, d.limit, d.offset);
    setError('err-traces', null);
    var items = d.items || [];
    if(!state.session && items.length > 0){state.session = items[0].session;state.entries = 0;}
    if(state.session){loadEntries();}
  }).catch(function(e){setError('err-traces', e);});
}
function loadEntries(){
  if(!state.session){return;}
  setText('trace-title', 'Pasos de la sesión ' + state.session);
  fetchData('/v1/traces?session=' + encodeURIComponent(state.session) + '&limit=' + PAGE + '&offset=' + state.entries).then(function(d){
    var tb = document.getElementById('tb-entries');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    (d.items || []).forEach(function(e){
      var tr = document.createElement('tr');
      tr.appendChild(td(e.call || ''));
      tr.appendChild(td(e.tool || ''));
      var msg = e.message || '';
      tr.appendChild(td(msg.length > 140 ? msg.slice(0, 140) + '…' : msg));
      tb.appendChild(tr);
    });
    pageInfo('info-entries', 'prev-entries', 'next-entries', d.total, d.limit, d.offset);
  }).catch(function(e){setError('err-traces', e);});
}
function loadBudget(){
  fetchData('/v1/budget?limit=' + PAGE + '&offset=' + state.budget).then(function(d){
    var tb = document.getElementById('tb-budget');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    (d.items || []).forEach(function(e){
      var tr = document.createElement('tr');
      tr.appendChild(td(String(e.timestamp || '').slice(0, 10)));
      tr.appendChild(td(e.kind || ''));
      tr.appendChild(td(e.task || ''));
      tr.appendChild(td(e.outcome || ''));
      tb.appendChild(tr);
    });
    pageInfo('info-budget', 'prev-budget', 'next-budget', d.total, d.limit, d.offset);
    setError('err-budget', null);
  }).catch(function(e){setError('err-budget', e);});
}
function loadAll(){loadHeader();loadCards();loadGraphs();loadReceipts();loadObs();loadLessons();loadTraces();loadBudget();}
function bindPager(key, prevId, nextId, reload){
  document.getElementById(prevId).addEventListener('click', function(){
    state[key] = Math.max(0, state[key] - PAGE);reload();
  });
  document.getElementById(nextId).addEventListener('click', function(){
    state[key] = state[key] + PAGE;reload();
  });
}
bindPager('receipts', 'prev-receipts', 'next-receipts', loadReceipts);
bindPager('obs', 'prev-obs', 'next-obs', loadObs);
bindPager('lessons', 'prev-lessons', 'next-lessons', loadLessons);
bindPager('traces', 'prev-traces', 'next-traces', loadTraces);
bindPager('entries', 'prev-entries', 'next-entries', loadEntries);
bindPager('budget', 'prev-budget', 'next-budget', loadBudget);
document.getElementById('go-obs').addEventListener('click', function(){state.obs = 0;loadObs();});
document.getElementById('go-lessons').addEventListener('click', function(){state.lessons = 0;loadLessons();});
document.getElementById('refresh').addEventListener('click', loadAll);
loadAll();setInterval(loadAll, 30000);
</script></body></html>`
