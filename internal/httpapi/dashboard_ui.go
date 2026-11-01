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
:root{
  /* Brand */
  --regulus-brand:#D9A441;
  --regulus-brand-bright:#F2C66D;
  --regulus-brand-dark:#B96C32;
  /* Backgrounds */
  --regulus-bg:#0C1018;
  --regulus-surface:#171E29;
  --regulus-surface-raised:#202A38;
  /* Typography */
  --regulus-text:#E7EAF0;
  --regulus-muted:#8290A3;
  /* Borders */
  --regulus-border:#303B4B;
  /* Modules */
  --module-sextant:#5B9BD5;
  --module-gauntlet:#E69A42;
  --module-guardians:#55B889;
  --module-decision:#A78BFA;
  --module-almanac:#48BFC0;
  --module-evolution:#F2C66D;
  --module-observability:#8290A3;
  /* Semantic statuses */
  --status-pass:#55B889;
  --status-fail:#E16B68;
  --status-blocked:#E69A42;
  --status-inconclusive:#8290A3;
  /* Typography (con fallbacks del sistema) */
  --font-heading:"Space Grotesk","Inter",system-ui,-apple-system,"Segoe UI",sans-serif;
  --font-body:"Inter",system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
  --font-mono:"JetBrains Mono",ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
  /* Shape */
  --radius-sm:4px;
  --radius-md:8px;
  --radius-lg:12px;
  --radius-xl:16px;
  /* Borders */
  --border-width:1px;
  /* Motion */
  --transition-fast:150ms ease;
  --transition-normal:220ms ease;
}
*{box-sizing:border-box}
body{font-family:var(--font-body);margin:0;background:var(--regulus-bg);color:var(--regulus-text)}
header.top{background:var(--regulus-bg);color:var(--regulus-text);padding:28px 24px;border-bottom:2px solid var(--regulus-brand)}
header.top .wrap,main{max-width:1100px;margin:0 auto}
header.top h1{margin:0 0 8px;font-size:28px;font-family:var(--font-heading);color:var(--regulus-brand-bright);letter-spacing:.01em}
header.top p{margin:4px 0;color:var(--regulus-muted)}
#status,#health{margin-top:12px;display:inline-block;background:var(--regulus-surface);border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-md);padding:8px 14px;font-size:14px;margin-right:8px;color:var(--regulus-text)}
main{padding:24px}
section{background:var(--regulus-surface);border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-lg);padding:20px;margin:0 0 20px}
h2,h3{font-family:var(--font-heading)}
h2{margin:0 0 4px;font-size:20px;color:var(--regulus-text)}
p.plain{color:var(--regulus-muted);margin:6px 0 14px;font-size:14px}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}
.card{border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-md);padding:14px;background:var(--regulus-surface-raised)}
.card h3{margin:0 0 4px;font-size:15px;color:var(--regulus-text)}
.card .num{font-size:32px;font-weight:700;color:var(--regulus-brand-bright);font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.card p{margin:6px 0 0;font-size:13px;color:var(--regulus-muted)}
.bar-row{display:grid;grid-template-columns:180px 1fr 60px;gap:8px;align-items:center;margin:6px 0;font-size:13px}
.bar-track{background:var(--regulus-bg);border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-sm);height:14px;overflow:hidden}
.bar-fill{height:100%;background:var(--regulus-brand);border-radius:var(--radius-sm);transition:width var(--transition-normal)}
.bar-fill.fill-pass{background:var(--status-pass)}
.bar-fill.fill-fail{background:var(--status-fail)}
.bar-fill.fill-blocked{background:var(--status-blocked)}
.bar-fill.fill-inconclusive{background:var(--status-inconclusive)}
.bar-row .v{text-align:right;font-variant-numeric:tabular-nums;font-family:var(--font-mono)}
table{width:100%;border-collapse:collapse;font-size:14px}
th,td{text-align:left;padding:8px;border-bottom:var(--border-width) solid var(--regulus-border);vertical-align:top}
th{background:var(--regulus-surface-raised);font-size:12px;text-transform:uppercase;letter-spacing:.04em;color:var(--regulus-muted);font-family:var(--font-body)}
td.num,.t-mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.pager{display:flex;gap:8px;align-items:center;margin-top:10px;font-size:13px;color:var(--regulus-muted)}
button{border:var(--border-width) solid var(--regulus-border);background:var(--regulus-surface-raised);color:var(--regulus-text);border-radius:var(--radius-md);padding:6px 12px;cursor:pointer;font-size:14px;font-family:var(--font-body);transition:background var(--transition-fast),border-color var(--transition-fast)}
button:hover{border-color:var(--regulus-brand)}
button:disabled{opacity:.45;cursor:default}
button.primary{background:var(--regulus-brand);color:var(--regulus-bg);border-color:var(--regulus-brand)}
.controls{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:10px}
input,select{border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-md);padding:6px 10px;font-size:14px;background:var(--regulus-surface-raised);color:var(--regulus-text);font-family:var(--font-body)}
.verdict-pass,.ok{color:var(--status-pass);font-weight:600}
.verdict-fail,.fail{color:var(--status-fail);font-weight:600}
.verdict-blocked{color:var(--status-blocked);font-weight:600}
.verdict-inconclusive{color:var(--status-inconclusive);font-weight:600}
.verdict-pass,.verdict-fail,.verdict-blocked,.verdict-inconclusive,.ok,.fail{display:inline-block;border:var(--border-width) solid currentColor;border-radius:var(--radius-xl);padding:1px 10px;font-size:13px}
.err{color:var(--status-fail);font-size:13px}
footer{color:var(--regulus-muted);font-size:13px;text-align:center;padding:0 0 28px}
details{margin-top:8px;font-size:13px}
summary{cursor:pointer}
pre{background:var(--regulus-bg);color:var(--regulus-text);border:var(--border-width) solid var(--regulus-border);padding:12px;border-radius:var(--radius-md);overflow:auto;font-size:12px;font-family:var(--font-mono)}
a{color:var(--regulus-brand-bright)}
:focus-visible{outline:2px solid var(--regulus-brand-bright);outline-offset:2px;border-radius:var(--radius-sm)}
@media (prefers-reduced-motion:reduce){
  *{transition:none;animation:none}
  .bar-fill{transition:none}
}
@media (max-width:640px){
  .bar-row{grid-template-columns:1fr;gap:4px}
  .bar-row .v{text-align:left}
}
</style>
</head><body>
<header class="top"><div class="wrap">
<h1 id="project-name">Regulus — Qué hay guardado en tu proyecto</h1>
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
function verdictFill(label){
  var v = String(label || '').toLowerCase();
  if(v === 'pass' || v === 'confirmadas' || v === 'confirmada'){return ' fill-pass';}
  if(v === 'fail'){return ' fill-fail';}
  if(v === 'blocked' || v === 'bloqueado' || v === 'por confirmar'){return ' fill-blocked';}
  if(v === 'sin datos' || v === 'inconclusive'){return ' fill-inconclusive';}
  return '';
}
function barRow(box, label, value, max){
  var row = document.createElement('div');row.className = 'bar-row';
  var l = document.createElement('span');l.textContent = label;row.appendChild(l);
  var track = document.createElement('div');track.className = 'bar-track';
  var fill = document.createElement('div');fill.className = 'bar-fill' + verdictFill(label);
  fill.style.width = (max > 0 ? Math.round(100 * value / max) : 0) + '%';
  track.appendChild(fill);row.appendChild(track);
  var v = document.createElement('span');v.className = 'v';v.textContent = String(value);row.appendChild(v);
  box.appendChild(row);
}
function barGroup(box, title, pairs, source){
  var h = document.createElement('h3');h.textContent = title;box.appendChild(h);
  if(source){var src = document.createElement('p');src.className = 'plain';src.textContent = source;box.appendChild(src);}
  var max = 0;
  pairs.forEach(function(p){if(p[1] > max){max = p[1];}});
  if(pairs.length === 0){
    var e = document.createElement('p');e.className = 'plain';e.textContent = 'Sin datos todavía: proyecto vacío o filtro sin resultados.';box.appendChild(e);return;
  }
  pairs.forEach(function(p){barRow(box, p[0], p[1], max);});
}
function emptyRow(tb, cols, msg){
  var tr = document.createElement('tr');
  var c = document.createElement('td');c.colSpan = cols;c.textContent = msg;tr.appendChild(c);
  tb.appendChild(tr);
}
function loadHeader(){
  Promise.all([fetchData('/v1/status'), fetchData('/v1/version'), fetchData('/v1/metrics')]).then(function(r){
    var st = r[0], ver = r[1], me = r[2];
    var ok = st.exit === 0;
    var project = String(st.project || '').trim();
    if(project){
      document.title = 'Regulus — ' + project;
      var h1 = document.getElementById('project-name');
      if(h1){h1.textContent = 'Regulus — Qué hay guardado en ' + project;}
    }
    setText('status', 'Estado del proyecto ' + (project ? project + ' ' : '') + '(GET /v1/status campos exit/project + GET /v1/version): ' + (ok ? 'bien' : 'revisar (código ' + st.exit + ')') + ' · ' + (ver.version || '').trim());
    setText('status-full', (st.stdout || '') + (st.stderr ? '\n' + st.stderr : ''));
    var m = me.metrics || {};
    var evo = 'Mejoras usadas hoy (GET /v1/metrics evolution_used/cap): ' + me.evolution_used + ' de ' + me.evolution_cap;
    var pass = m.verification_pass_rate;
    setText('health', 'Salud del sistema (GET /v1/metrics: tasks, receipts_pass/receipts_total): ' + m.tasks + ' tareas, ' + m.receipts_pass + ' de ' + m.receipts_total + ' comprobaciones bien' + (pass ? ' (' + Math.round(pass * 100) + '%)' : '') + ' · ' + evo);
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
    box.appendChild(card('Guardado', r[0].total, 'Cosas guardadas en total. Fuente: GET /v1/almanac/summary campo total, sin filtro.'));
    box.appendChild(card('Evidencia', r[1].total, 'Comprobantes de verificación. Fuente: GET /v1/receipts campo total, sin filtro (la tarjeta pide ?limit=1 pero el total es global).'));
    box.appendChild(card('Memoria', r[2].total, 'Recuerdos anotados. Fuente: GET /v1/observations campo total, sin filtro q/type (la tabla sí filtra).'));
    var lt = (r[3].total !== undefined) ? r[3].total : 0;
    box.appendChild(card('Aprendizaje', lt, 'Lecciones confirmadas + por confirmar. Fuente: GET /v1/lessons?status=all campo total (admitidas + candidatas).'));
    box.appendChild(card('Coste', r[4].total, 'Movimientos del presupuesto. Fuente: GET /v1/budget campo total, sin filtro (la tarjeta pide ?limit=1 pero el total es global).'));
    setError('cards-error', null);
  }).catch(function(e){setError('cards-error', e);});
}
function loadGraphs(){
  Promise.all([
    fetchData('/v1/almanac/summary'),
    fetchData('/v1/budget?limit=200'),
    fetchData('/v1/receipts?limit=200')
  ]).then(function(r){
    var box = document.getElementById('graphs');
    while(box.firstChild){box.removeChild(box.firstChild);}
    var byType = [];
    var bt = r[0].by_type || {};
    Object.keys(bt).sort().forEach(function(k){byType.push([k, bt[k]]);});
    barGroup(box, 'Recuerdos por tipo', byType, 'Fuente: GET /v1/almanac/summary campo by_type (conteo global, sin límite).');
    var adm = r[0].lessons_admitted || 0, cand = r[0].candidates_pending || 0;
    barGroup(box, 'Lecciones confirmadas frente a por confirmar', [['Confirmadas', adm], ['Por confirmar', cand]], 'Fuente: GET /v1/almanac/summary campos lessons_admitted + candidates_pending (global).');
    var byKind = {};
    (r[1].items || []).forEach(function(e){var k = e.kind || '?';byKind[k] = (byKind[k] || 0) + 1;});
    var kinds = Object.keys(byKind).sort().map(function(k){return [k, byKind[k]];});
    barGroup(box, 'Presupuesto usado por tipo', kinds, 'Fuente: GET /v1/budget?limit=200 — agrupa kind de los primeros 200 de ' + r[1].total + ' (total). Si el total supera 200 el gráfico es parcial.');
    var byVerdict = {};
    (r[2].items || []).forEach(function(it){
      var v = (it.receipt && it.receipt.verdict) || '?';
      byVerdict[v] = (byVerdict[v] || 0) + 1;
    });
    var verdicts = Object.keys(byVerdict).sort().map(function(k){return [k, byVerdict[k]];});
    barGroup(box, 'Comprobaciones por resultado', verdicts, 'Fuente: GET /v1/receipts?limit=200 — agrupa receipt.verdict de los primeros 200 de ' + r[2].total + ' (total, los más antiguos primero). Si el total supera 200 el gráfico es parcial.');
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
      var raw = String(rc.verdict || '').toLowerCase();
      if(raw === 'pass'){s.textContent = 'PASS';s.className = 'verdict-pass';}
      else if(raw === 'fail'){s.textContent = 'FAIL';s.className = 'verdict-fail';}
      else if(raw === 'blocked'){s.textContent = 'BLOCKED';s.className = 'verdict-blocked';}
      else{s.textContent = rc.verdict ? String(rc.verdict).toUpperCase() : 'Sin dato';s.className = 'verdict-inconclusive';}
      v.appendChild(s);tr.appendChild(v);
      var started = rc.started_at || rc.startedAt || '';
      tr.appendChild(td(String(started).slice(0, 19).replace('T', ' ')));
      tb.appendChild(tr);
    });
    if((d.items || []).length === 0){emptyRow(tb, 4, 'Sin comprobantes todavía: proyecto vacío o filtro sin resultados (GET /v1/receipts, total 0).');}
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
    if((d.items || []).length === 0){emptyRow(tb, 3, 'Sin recuerdos todavía: proyecto vacío o búsqueda sin resultados (GET /v1/observations, total 0).');}
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
      tag.className = l.status === 'candidate' ? 'verdict-blocked' : 'verdict-pass';
      st.appendChild(tag);tr.appendChild(st);
      var txt = l.lesson || l.cue || '';
      tr.appendChild(td(txt.length > 140 ? txt.slice(0, 140) + '…' : txt));
      tb.appendChild(tr);
    });
    if((d.items || []).length === 0){emptyRow(tb, 3, 'Sin lecciones todavía: proyecto vacío o filtro sin resultados (GET /v1/lessons, total 0; filtro status=' + s + ').');}
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
    if((d.items || []).length === 0){emptyRow(tb, 3, 'Sin sesiones todavía: proyecto sin trazas (GET /v1/traces, total 0).');}
    pageInfo('info-traces', 'prev-traces', 'next-traces', d.total, d.limit, d.offset);
    setError('err-traces', null);
    var items = d.items || [];
    if(!state.session && items.length > 0){state.session = items[0].session;state.entries = 0;}
    if(state.session){loadEntries();}
    else{
      setText('trace-title', 'Pasos de la sesión — sin sesión (proyecto vacío o sin trazas)');
      var etb = document.getElementById('tb-entries');
      while(etb.firstChild){etb.removeChild(etb.firstChild);}
      emptyRow(etb, 3, 'Sin pasos: no hay sesión seleccionada (GET /v1/traces?session=..., total 0).');
      pageInfo('info-entries', 'prev-entries', 'next-entries', 0, PAGE, 0);
    }
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
    if((d.items || []).length === 0){emptyRow(tb, 3, 'Sin pasos en esta sesión (GET /v1/traces?session=' + state.session + ', total 0).');}
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
    if((d.items || []).length === 0){emptyRow(tb, 4, 'Sin movimientos todavía: presupuesto sin usar (GET /v1/budget, total 0).');}
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
