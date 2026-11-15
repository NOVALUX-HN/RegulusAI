var PAGE = 10;
var state = {receipts:0, obs:0, lessons:0, traces:0, entries:0, budget:0, session:'', project:''};
function fetchData(path){
  if(state.project && path !== '/v1/projects'){
    path += (path.indexOf('?') >= 0 ? '&' : '?') + 'project=' + encodeURIComponent(state.project);
  }
  return fetch(path).then(function(r){return r.json();}).then(function(env){
    if(!env || env.ok !== true || !env.data){throw new Error('respuesta inesperada');}
    return env.data;
  });
}
function setText(id, s){document.getElementById(id).textContent = s;}
function setError(id, e){setText(id, e ? 'No se pudo cargar: ' + (e.message || e) : '');}
function td(s){var c = document.createElement('td');c.textContent = s;return c;}
function tdNum(s){var c = td(s);c.className = 'num';return c;}
function tdClip(s){var c = document.createElement('td');s = String(s);c.textContent = s.length > 140 ? s.slice(0, 140) + '…' : s;if(s.length > 140){c.title = s;}return c;}
function verdictPill(v){
  var s = document.createElement('span');
  var raw = String(v || '').toLowerCase();
  if(raw === 'pass'){s.textContent = 'PASS';s.className = 'verdict-pass';}
  else if(raw === 'fail'){s.textContent = 'FAIL';s.className = 'verdict-fail';}
  else if(raw === 'blocked'){s.textContent = 'BLOCKED';s.className = 'verdict-blocked';}
  else{s.textContent = v ? String(v).toUpperCase() : 'Sin dato';s.className = 'verdict-inconclusive';}
  return s;
}
function pageInfo(infoId, prevId, nextId, total, limit, offset, source){
  var lo = total === 0 ? 0 : offset + 1;
  var hi = Math.min(offset + limit, total);
  var page = Math.floor(offset / limit) + 1;
  var pages = Math.max(1, Math.ceil(total / limit));
  var msg = 'Mostrando ' + lo + '–' + hi + ' de ' + total + ' · página ' + page + ' de ' + pages;
  if(source){msg += ' · ' + source;}
  setText(infoId, msg);
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
    var totals = [r[0].total || 0, r[1].total || 0, r[2].total || 0, lt, r[4].total || 0];
    var none = true;
    totals.forEach(function(n){if(n > 0){none = false;}});
    var emptyNote = document.getElementById('cards-empty');
    if(emptyNote){
      if(none){emptyNote.hidden = false;emptyNote.textContent = 'Sin datos todavía: proyecto vacío (GET /v1/almanac/summary y colecciones con total 0).';}
      else{emptyNote.hidden = true;emptyNote.textContent = '';}
    }
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
/* V1 visión: todo se deriva en cliente de las colecciones /v1/* existentes
   (cap 1000 por colección; "parcial" si el total lo supera). Sin ramas ni
   #secuencial: no existen en stores. */
var VISION_CAP = 1000;
var VISION_PAGE = 200;
/* S18: cada KPI muestra su estado en vez de solo "…/cargando": cero (0
   medido), desconocido (sin generated_at del agregado), no disponible
   (fetch fallido) o stale (>max-age 10s del Cache-Control) y error.
   El title de cada KPI cita fuente + fórmula + generated_at. */
var VISION_MAX_AGE_MS = 1000 * 10;
function visionFreshness(generatedAt){
  var t = visionParseTime(generatedAt);
  if(t === null){return 'desconocido';}
  return (Date.now() - t > VISION_MAX_AGE_MS) ? 'stale' : 'fresco';
}
function visionKpiTitle(id, source, formula, generatedAt){
  var el = document.getElementById(id);
  if(!el){return;}
  el.title = 'Fuente: ' + source + '; fórmula: ' + formula + '; generado: ' + (generatedAt || 'desconocido') + ' (' + visionFreshness(generatedAt) + ')';
}
function visionKpiUnavailable(){
  ['kpi-evidencias-val', 'kpi-ejecuciones-val', 'kpi-pass-val', 'kpi-lecciones-val', 'kpi-ramas-val'].forEach(function(id){setText(id, 'no disponible');});
}
/* V4: trae los NUEVOS (cola): las ventanas de 7 días, los deltas y el
   último ciclo viven en lo más reciente. Si el total supera el cap, los
   más antiguos quedan fuera y se avisa "parcial". */
function visionFetchAll(base){
  var sep = base.indexOf('?') >= 0 ? '&' : '?';
  return fetchData(base + sep + 'limit=1&offset=0').then(function(head){
    var total = (head && head.total) || 0;
    if(total <= 0){return {items: [], total: 0, partial: false};}
    var want = Math.min(total, VISION_CAP);
    if(total <= VISION_PAGE){
      return fetchData(base + sep + 'limit=' + VISION_PAGE + '&offset=0').then(function(first){
        return {items: (first.items || []).slice(0), total: total, partial: false};
      });
    }
    var start = total - want;
    var chain = Promise.resolve([]);
    for(var off = start; off < total; off += VISION_PAGE){
      (function(o){
        chain = chain.then(function(acc){
          return fetchData(base + sep + 'limit=' + VISION_PAGE + '&offset=' + o).then(function(d){
            return acc.concat(d.items || []);
          });
        });
      })(off);
    }
    return chain.then(function(all){
      return {items: all.slice(0, VISION_CAP), total: total, partial: total > VISION_CAP};
    });
  });
}
function visionParseTime(s){
  if(!s){return null;}
  var t = Date.parse(s);
  if(isNaN(t)){return null;}
  return t;
}
function visionDayKey(d){return d.getFullYear() + '-' + (d.getMonth() + 1) + '-' + d.getDate();}
var VISION_MONTHS = ['ene','feb','mar','abr','may','jun','jul','ago','sep','oct','nov','dic'];
function visionDayLabel(d){return d.getDate() + ' ' + VISION_MONTHS[d.getMonth()];}
function visionPad(n){return (n < 10 ? '0' : '') + n;}
function visionDateTime(t){
  var d = new Date(t);
  return visionPad(d.getDate()) + ' ' + VISION_MONTHS[d.getMonth()] + ', ' + visionPad(d.getHours()) + ':' + visionPad(d.getMinutes());
}
function visionRel(t){
  var s = Math.max(0, Math.round((Date.now() - t) / 1000));
  if(s < 60){return 'ahora mismo';}
  var m = Math.floor(s / 60);
  if(m < 60){return 'Hace ' + m + ' min';}
  var h = Math.floor(m / 60);
  if(h < 24){return 'Hace ' + h + ' h';}
  return 'hace ' + Math.floor(h / 24) + ' d';
}
function visionFmtDur(ms){
  var s = Math.max(0, Math.round((ms || 0) / 1000));
  if(s < 60){return s + ' s';}
  var m = Math.floor(s / 60);
  if(m < 60){return m + ' min ' + visionPad(s % 60) + ' s';}
  return Math.floor(m / 60) + ' h ' + visionPad(m % 60) + ' min';
}
function visionLast7(){
  var days = [];
  var now = new Date();
  for(var i = 6; i >= 0; i--){
    var d = new Date(now.getFullYear(), now.getMonth(), now.getDate() - i);
    days.push(d);
  }
  return days;
}
function visionWindowCounts(times, days){
  var keys = {};
  days.forEach(function(d){keys[visionDayKey(d)] = 0;});
  var lastStart = days[0].getTime();
  var prevStart = lastStart - 7 * 86400000;
  var last = 0, prev = 0;
  times.forEach(function(t){
    if(t === null || t === undefined){return;}
    var k = visionDayKey(new Date(t));
    if(keys[k] !== undefined){keys[k]++;}
    if(t >= lastStart){last++;}
    else if(t >= prevStart){prev++;}
  });
  return {perDay: days.map(function(d){return keys[visionDayKey(d)];}), last: last, prev: prev};
}
function visionFmtInt(n){
  var v = Number(n);
  if(!isFinite(v)){return '0';}
  return Math.round(v).toLocaleString('en-US');
}
function visionFmtPct1(x){
  var v = Number(x);
  if(!isFinite(v)){v = 0;}
  return (Math.round(v * 10) / 10).toFixed(1) + '%';
}
function visionPctDelta(last, prev){
  if(last === 0 && prev === 0){return {text: 'sin cambios (7 días)', cls: ''};}
  if(prev === 0){return {text: 'nuevo en 7 días (+' + visionFmtInt(last) + ')', cls: 'delta-up'};}
  var p = Math.round(100 * (last - prev) / prev);
  var sign = p > 0 ? '+' : (p < 0 ? '−' : '');
  return {text: sign + visionFmtInt(Math.abs(p)) + '% (7 días)', cls: p > 0 ? 'delta-up' : (p < 0 ? 'delta-down' : '')};
}
function visionDeltaText(last, prev, suffix){
  var diff = last - prev;
  var base = suffix || ' (7 días)';
  if(last === 0 && prev === 0){return {text: 'sin cambios' + base, cls: ''};}
  var sign = diff > 0 ? '+' : (diff < 0 ? '−' : '');
  return {text: sign + String(Math.abs(diff)).replace('-', '') + base, cls: diff > 0 ? 'delta-up' : (diff < 0 ? 'delta-down' : '')};
}
function svgEl(tag, attrs, parent, ns){
  var el = document.createElementNS('http://www.w3.org/2000/svg', tag);
  if(attrs){Object.keys(attrs).forEach(function(k){el.setAttribute(k, attrs[k]);});}
  if(parent){parent.appendChild(el);}
  return el;
}
function svgText(parent, x, y, s, anchor){
  var t = svgEl('text', {x: x, y: y, 'text-anchor': anchor || 'middle'}, parent);
  t.textContent = s;
  return t;
}
function visionClearSvg(id){
  var svg = document.getElementById(id);
  var keep = null;
  for(var i = 0; i < svg.childNodes.length; i++){
    var c = svg.childNodes[i];
    if(c.nodeType === 1 && c.tagName && String(c.tagName).toLowerCase() === 'title'){keep = c;break;}
  }
  while(svg.firstChild){svg.removeChild(svg.firstChild);}
  if(keep){svg.appendChild(keep);}
  return svg;
}
function visionDrawBars(days, counts){
  var svg = visionClearSvg('chart-bars');
  svg.setAttribute('viewBox', '0 0 560 220');
  var W = 560, H = 220, L = 44, B = 28, T = 14, R = 10;
  var max = 0;
  counts.forEach(function(c){if(c > max){max = c;}});
  var top = max > 0 ? Math.ceil(max * 1.15) : 1;
  if(top < 4){top = 4;}
  var steps = 4, n = days.length;
  var iw = (W - L - R) / n, ih = (H - T - B);
  function y(v){return T + ih - (ih * v / top);}
  for(var g = 0; g <= steps; g++){
    var v = Math.round(top * g / steps);
    svgEl('line', {'class': 'grid-line', x1: L, y1: y(v), x2: W - R, y2: y(v)}, svg);
    svgText(svg, L - 6, y(v) + 3, String(v), 'end');
  }
  svgEl('line', {'class': 'axis-line', x1: L, y1: T, x2: L, y2: H - B}, svg);
  svgEl('line', {'class': 'axis-line', x1: L, y1: H - B, x2: W - R, y2: H - B}, svg);
  for(var i = 0; i < n; i++){
    var cx = L + iw * i + iw / 2;
    if(counts[i] > 0){
      var bw = Math.min(48, iw * 0.62);
      var r = svgEl('rect', {x: cx - bw / 2, y: y(counts[i]), width: bw, height: Math.max(1, (H - B) - y(counts[i])), rx: 3, fill: '#D9A441'}, svg);
      r.appendChild(document.createElementNS('http://www.w3.org/2000/svg', 'title')).textContent = counts[i] + ' el ' + visionDayLabel(days[i]);
      svgText(svg, cx, y(counts[i]) - 5, String(counts[i]), 'middle');
    }
    svgText(svg, cx, H - 10, visionDayLabel(days[i]), 'middle');
  }
  if(max === 0){
    svgText(svg, L + (W - L - R) / 2, T + ih / 2, 'Sin datos todavía: no hay comprobantes en los últimos 7 días.', 'middle');
  }
  setText('chart-bars-title', 'Ejecuciones por día: ' + counts.join(', '));
}
function visionDrawLine(days, rates){
  var svg = visionClearSvg('chart-pass');
  svg.setAttribute('viewBox', '0 0 560 220');
  var W = 560, H = 220, L = 44, B = 28, T = 14, R = 10;
  var n = days.length, iw = (W - L - R) / n, ih = (H - T - B);
  var vals = rates.filter(function(r){return r !== null;});
  if(vals.length === 0){
    svgText(svg, L + (W - L - R) / 2, T + ih / 2, 'Sin datos todavía: no hay comprobantes en los últimos 7 días.', 'middle');
    setText('chart-pass-title', 'Tasa de éxito por día: sin datos');
    return;
  }
  var min = Math.min.apply(null, vals);
  var lo = min >= 90 ? 90 : Math.max(0, Math.floor(min - 5));
  if(lo >= 100){lo = 90;}
  var span = 100 - lo;
  function y(v){return T + ih * (100 - v) / span;}
  function tickLabel(v){
    var r = Math.round(v * 10) / 10;
    return (r === Math.round(r) ? String(Math.round(r)) : String(r)) + '%';
  }
  for(var g = 0; g <= 5; g++){
    var v = lo + span * g / 5;
    svgEl('line', {'class': 'grid-line', x1: L, y1: y(v), x2: W - R, y2: y(v)}, svg);
    svgText(svg, L - 6, y(v) + 3, tickLabel(v), 'end');
  }
  svgEl('line', {'class': 'axis-line', x1: L, y1: T, x2: L, y2: H - B}, svg);
  svgEl('line', {'class': 'axis-line', x1: L, y1: H - B, x2: W - R, y2: H - B}, svg);
  var seg = '', area = '', started = false, pts = [];
  for(var i = 0; i < n; i++){
    var cx = L + iw * i + iw / 2;
    svgText(svg, cx, H - 10, visionDayLabel(days[i]), 'middle');
    if(rates[i] === null){started = false;continue;}
    var cy = y(rates[i]);
    pts.push([cx, cy, rates[i], i]);
    if(!started){seg += 'M' + cx + ',' + cy;area = 'M' + cx + ',' + (H - B) + 'L' + cx + ',' + cy;started = true;}
    else{seg += 'L' + cx + ',' + cy;area += 'L' + cx + ',' + cy;}
  }
  var any = pts.length > 0;
  if(any){
    var last = pts[pts.length - 1];
    area += 'L' + last[0] + ',' + (H - B) + 'Z';
    svgEl('path', {d: area, fill: '#D9A441', opacity: '0.15', stroke: 'none'}, svg);
    svgEl('path', {d: seg, fill: 'none', stroke: '#D9A441', 'stroke-width': '2'}, svg);
    pts.forEach(function(p){
      var c = svgEl('circle', {cx: p[0], cy: p[1], r: 4, fill: '#F2C66D', stroke: '#0C1018'}, svg);
      c.appendChild(document.createElementNS('http://www.w3.org/2000/svg', 'title')).textContent = p[2] + '% el ' + visionDayLabel(days[p[3]]);
    });
  } else {
    svgText(svg, L + (W - L - R) / 2, T + ih / 2, 'Sin datos todavía: no hay comprobantes en los últimos 7 días.', 'middle');
  }
  var summary = rates.map(function(r){return r === null ? 'sin datos' : r + '%';}).join(', ');
  setText('chart-pass-title', 'Tasa de éxito por día: ' + summary);
}
/* V4: agrupa comprobantes como el backend (V3): mismo run_id = un ciclo;
   sin run_id = misma task + hora UTC. Ordena por primer started_at; el
   último grupo es el último ciclo. */
function visionGroupCycles(recItems){
  var byKey = {}, order = [];
  (recItems || []).forEach(function(it){
    var rc = it.receipt || {};
    var t = visionParseTime(rc.started_at);
    var key;
    if(rc.run_id){
      key = 'run:' + rc.run_id;
    } else {
      var task = it.task || rc.task || '';
      var h = t === null ? '?' : new Date(t).toISOString().slice(0, 13);
      key = 'legacy:' + task + '|' + h;
    }
    var g = byKey[key];
    if(!g){g = {key: key, task: it.task || rc.task || '', first: t, recs: []};byKey[key] = g;order.push(g);}
    g.recs.push(it);
    if(t !== null && (g.first === null || t < g.first)){g.first = t;}
  });
  order.sort(function(a, b){
    var fa = a.first === null ? 0 : a.first, fb = b.first === null ? 0 : b.first;
    if(fa === fb){return a.key < b.key ? -1 : 1;}
    return fa - fb;
  });
  return order;
}
function visionSetGateState(gate, verdict){
  var el = document.getElementById('cfg-' + gate);
  if(!el){return;}
  var raw = String(verdict || '').toLowerCase();
  if(raw === 'pass'){el.textContent = 'PASS';el.className = 'st verdict-pass';}
  else if(raw === 'fail'){el.textContent = 'FAIL';el.className = 'st verdict-fail';}
  else{el.textContent = '— en este ciclo';el.className = 'st verdict-inconclusive';}
}
/* V4: los ciclos de /v1/executions con el mismo cap que el resto
   (límite 200 por página del backend): pagina hasta 1000 y, si hay más,
   trae aparte el último (offset total-1) para que el pill #N sea exacto. */
function visionFetchExecutions(){
  var per = VISION_PAGE;
  return fetchData('/v1/executions?limit=' + per + '&offset=0').then(function(first){
    var total = first.total || 0;
    var items = (first.items || []).slice(0);
    if(total <= 0){return {items: [], total: 0, latest: null, partial: false};}
    if(total <= items.length){return {items: items, total: total, latest: items[items.length - 1], partial: false};}
    var want = Math.min(total, VISION_CAP);
    var chain = Promise.resolve(items);
    for(var off = items.length; off < want; off += per){
      (function(o){
        chain = chain.then(function(acc){
          return fetchData('/v1/executions?limit=' + per + '&offset=' + o).then(function(d){
            return acc.concat(d.items || []);
          });
        });
      })(off);
    }
    return chain.then(function(all){
      all = all.slice(0, VISION_CAP);
      function done(latest){return {items: all, total: total, latest: latest, partial: total > VISION_CAP};}
      if(total <= VISION_CAP){return done(all.length > 0 ? all[all.length - 1] : null);}
      return fetchData('/v1/executions?limit=1&offset=' + (total - 1)).then(function(t){
        return done((t.items || [])[0] || null);
      });
    });
  });
}
function loadVision(){
  Promise.all([
    fetchData('/v1/status'),
    fetchData('/v1/metrics'),
    fetchData('/v1/almanac/summary'),
    fetchData('/v1/projects'),
    fetchData('/v1/git'),
    visionFetchExecutions().then(function(ex){
      if(ex.latest && ex.latest.task){
        return visionFetchAll('/v1/receipts?task=' + encodeURIComponent(ex.latest.task)).then(function(tr){
          return {execs: ex, taskReceipts: tr};
        }, function(){return {execs: ex, taskReceipts: {items: []}};});
      }
      return {execs: ex, taskReceipts: {items: []}};
    }),
    visionFetchAll('/v1/receipts'),
    visionFetchAll('/v1/lessons?status=all')
  ]).then(function(r){
    var st = r[0], me = r[1], sum = r[2], projs = r[3];
    var git = r[4], bundle = r[5], receipts = r[6], lessons = r[7];
    var execs = (bundle && bundle.execs) || {items: [], total: 0, latest: null};
    var taskReceipts = (bundle && bundle.taskReceipts) || {items: []};
    setError('err-vision', null);
    var partials = [];
    if(receipts.partial){partials.push('receipts ' + receipts.items.length + ' de ' + receipts.total);}
    if(lessons.partial){partials.push('lessons ' + lessons.items.length + ' de ' + lessons.total);}
    if(execs && execs.total > (execs.items || []).length){partials.push('executions ' + (execs.items || []).length + ' de ' + execs.total);}
    var note = document.getElementById('vision-partial');
    if(note){
      if(partials.length > 0){note.hidden = false;note.textContent = 'Cifras de 7 días parciales (límite 1000 por colección): ' + partials.join(' · ') + '. Los totales de las tarjetas son globales.';}
      else{note.hidden = true;note.textContent = '';}
    }
    var m = me.metrics || {};
    var days = visionLast7();
    var rTimes = [];
    (receipts.items || []).forEach(function(it){
      var rc = it.receipt || {};
      var t = visionParseTime(rc.started_at);
      if(t !== null){rTimes.push(t);}
    });
    var rw = visionWindowCounts(rTimes, days);
    var perDayPass = [], perDayTotal = [];
    days.forEach(function(){perDayPass.push(0);perDayTotal.push(0);});
    (receipts.items || []).forEach(function(it){
      var rc = it.receipt || {};
      var t = visionParseTime(rc.started_at);
      if(t === null){return;}
      var k = visionDayKey(new Date(t));
      for(var i = 0; i < days.length; i++){
        if(visionDayKey(days[i]) === k){
          perDayTotal[i]++;
          if(String(rc.verdict).toLowerCase() === 'pass'){perDayPass[i]++;}
          break;
        }
      }
    });
    function rateWindow(from, to){
      var p = 0, q = 0;
      (receipts.items || []).forEach(function(it){
        var rc = it.receipt || {};
        var t = visionParseTime(rc.started_at);
        if(t === null || t < from || t >= to){return;}
        q++;
        if(String(rc.verdict).toLowerCase() === 'pass'){p++;}
      });
      return q > 0 ? 100 * p / q : null;
    }
    var lastStart = days[0].getTime(), prevStart = lastStart - 7 * 86400000, nowMs = Date.now();
    var rateLast = rateWindow(lastStart, nowMs), ratePrev = rateWindow(prevStart, lastStart);
    var lTimes = [];
    (lessons.items || []).forEach(function(l){
      var t = visionParseTime(l.CreatedAt || l.created_at);
      if(t !== null){lTimes.push(t);}
    });
    var lw = visionWindowCounts(lTimes, days);
    /* V4: ciclos desde /v1/executions (el backend numera) + rama de /v1/git.
       latestExec es el ciclo con # mayor (el más nuevo). */
    var execItems = (execs && execs.items) || [];
    var execTotal = (execs && execs.total) || 0;
    var latestExec = (execs && execs.latest) || null;
    /* V4: compuertas del último ciclo = receipts de SU task (filtro exacto
       del backend) agrupados igual que el backend; el último grupo es el
       ciclo #N del pill. Sin task no hay tabla fiable: se omite. */
    var lastCycle = null, lastCycleTask = null;
    if(latestExec && latestExec.task){
      var scoped = visionGroupCycles((taskReceipts.items || []).filter(function(it){
        var rc = it.receipt || {};
        return (it.task || rc.task || '') === latestExec.task;
      }));
      if(scoped.length > 0){lastCycle = scoped[scoped.length - 1];lastCycleTask = latestExec.task;}
    }
    var latestMs = null;
    if(latestExec && latestExec.date){latestMs = visionParseTime(latestExec.date);}
    var eTimes = [];
    execItems.forEach(function(e){
      var t = visionParseTime(e.date);
      if(t !== null){eTimes.push(t);}
    });
    var ew = visionWindowCounts(eTimes, days);
    var evTotal = m.receipts_total;
    if(evTotal === undefined || evTotal === null){evTotal = receipts.total;}
    setText('kpi-evidencias-val', visionFmtInt(evTotal));
    var evD = visionPctDelta(rw.last, rw.prev);
    var evDelta = document.getElementById('kpi-evidencias-delta');
    evDelta.textContent = evD.text;
    evDelta.className = 'delta ' + evD.cls;
    setText('kpi-ejecuciones-val', visionFmtInt(execTotal));
    var exD = visionPctDelta(ew.last, ew.prev);
    var exDelta = document.getElementById('kpi-ejecuciones-delta');
    exDelta.textContent = exD.text;
    exDelta.className = 'delta ' + exD.cls;
    var rate = m.verification_pass_rate;
    if(rate === undefined || rate === null){rate = 0;}
    setText('kpi-pass-val', visionFmtPct1(rate * 100));
    setText('kpi-pass-sub', visionFmtInt(m.receipts_pass || 0) + ' / ' + visionFmtInt(m.receipts_total || 0));
    var passDelta = document.getElementById('kpi-pass-delta');
    if(rateLast === null && ratePrev === null){passDelta.textContent = 'sin datos (7 días)';passDelta.className = 'delta';}
    else if(ratePrev === null){passDelta.textContent = (Math.round(rateLast * 10) / 10) + '% en 7 días (sin previo)';passDelta.className = 'delta';}
    else{
      var pp = Math.round((rateLast === null ? 0 : rateLast - ratePrev) * 10) / 10;
      passDelta.textContent = (pp > 0 ? '+' : (pp < 0 ? '−' : '')) + String(Math.abs(pp)).replace('-', '') + ' pp (7 días)';
      passDelta.className = 'delta ' + (pp > 0 ? 'delta-up' : (pp < 0 ? 'delta-down' : ''));
    }
    setText('kpi-lecciones-val', visionFmtInt(sum.lessons_admitted || 0));
    var lesD = visionDeltaText(lw.last, lw.prev, ' nuevas (7 días)');
    var lesDelta = document.getElementById('kpi-lecciones-delta');
    lesDelta.textContent = lesD.text;
    lesDelta.className = 'delta ' + lesD.cls;
    /* Ramas activas: distintas en ciclos + actual; sin manifest = "—"
       (no se inventa). Solo se ve este checkout: se avisa. */
    var branchSet = {};
    execItems.forEach(function(e){
      if(e.branch && e.branch !== '—'){branchSet[e.branch] = true;}
    });
    var gitBranch = (git && git.branch) || '—';
    if(gitBranch && gitBranch !== '—'){branchSet[gitBranch] = true;}
    var branchNames = Object.keys(branchSet).sort();
    setText('kpi-ramas-val', visionFmtInt(branchNames.length));
    setText('kpi-ramas-actual', 'actual: ' + gitBranch);
    var ramasDelta = document.getElementById('kpi-ramas-delta');
    if(branchNames.length <= 1){
      ramasDelta.textContent = 'solo se ve este checkout (GET /v1/git); históricos sin rama = —';
    } else {
      ramasDelta.textContent = branchNames.slice(0, 5).join(', ') + (branchNames.length > 5 ? '…' : '');
    }
    ramasDelta.className = 'delta';
    /* S18: title por KPI con fuente + fórmula + generated_at; sin
       generated_at el estado es desconocido, con >max-age es stale. */
    visionKpiTitle('kpi-evidencias-val', 'GET /v1/metrics receipts_total', 'total global; delta % 7 días desde started_at', me.generated_at);
    visionKpiTitle('kpi-ejecuciones-val', 'GET /v1/executions total', 'ciclos #N (1 = más antiguo); delta % 7 días desde date', undefined);
    visionKpiTitle('kpi-pass-val', 'GET /v1/metrics verification_pass_rate', 'pass/total global; delta en pp 7 días', me.generated_at);
    visionKpiTitle('kpi-lecciones-val', 'GET /v1/almanac/summary lessons_admitted', 'admitidas global; delta 7 días desde CreatedAt', sum.generated_at);
    visionKpiTitle('kpi-ramas-val', 'GET /v1/executions branch + GET /v1/git', 'distintas en ciclos + actual', git.generated_at);
    var pill = document.getElementById('last-cycle');
    if(latestExec === null){
      pill.className = 'pill';
      setText('last-cycle-text', 'Último ciclo: sin datos');
    } else {
      var okCycle = String(latestExec.result || '').toLowerCase() === 'pass';
      pill.className = 'pill ' + (okCycle ? 'is-pass' : 'is-fail');
      var pillDate = latestMs === null ? 'sin fecha' : visionDateTime(latestMs);
      setText('last-cycle-text', 'Último ciclo: ' + (okCycle ? 'PASS' : 'FAIL') + ' #' + latestExec.n + ' · ' + pillDate);
    }
    var plist = Array.isArray(projs) ? projs : [];
    var pname = String(st.project || '').trim() || (plist.length > 0 ? String(plist[0].name || '') : '');
    setText('proj-name', pname || '—');
    setText('proj-branch', gitBranch);
    setText('proj-activity', latestMs === null ? 'sin actividad' : visionRel(latestMs));
    visionDrawBars(days, rw.perDay);
    setError('err-bars', null);
    var rates = perDayTotal.map(function(tot, i){return tot > 0 ? Math.round(1000 * perDayPass[i] / tot) / 10 : null;});
    visionDrawLine(days, rates);
    setError('err-pass', null);
    /* V4: últimas ejecuciones desde /v1/executions (#/Rama/...), las más
       nuevas primero; # ordenado por el backend (1 = más antiguo). */
    var ordered = execItems.slice(0).sort(function(a, b){return (b.n || 0) - (a.n || 0);});
    var tb = document.getElementById('tb-runs');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    ordered.slice(0, 8).forEach(function(e){
      var tr = document.createElement('tr');
      tr.appendChild(td('#' + e.n));
      var bc = td(e.branch || '—');
      bc.title = e.task || '';
      tr.appendChild(bc);
      var v = document.createElement('td');
      v.appendChild(verdictPill(e.result || ''));
      tr.appendChild(v);
      tr.appendChild(td(visionFmtInt(e.gates_pass || 0) + ' / ' + visionFmtInt(e.gates_total || 0)));
      tr.appendChild(td(visionFmtDur(e.duration_ms || 0)));
      var ems = visionParseTime(e.date);
      tr.appendChild(td(ems === null ? 'sin dato' : visionDateTime(ems)));
      tb.appendChild(tr);
    });
    if(ordered.length === 0){emptyRow(tb, 6, 'Sin ejecuciones todavía: proyecto vacío (GET /v1/executions, total 0).');}
    setError('err-runs', null);
    /* V4: compuertas del último ciclo = último grupo de receipts con el
       mismo agrupado que el backend; filas numeradas con la evidencia. */
    var tg = document.getElementById('tb-lastgates');
    while(tg.firstChild){tg.removeChild(tg.firstChild);}
    ['typecheck', 'lint', 'build', 'unit-tests', 'coverage-floor', 'mutation', 'crap', 'vulncheck', 'spec', 'ts-typecheck', 'ts-unit-tests', 'gofmt'].forEach(function(g){visionSetGateState(g, '');});
    if(lastCycle === null){
      setText('last-task', '');
      emptyRow(tg, 5, 'Sin compuertas: el último ciclo no tiene comprobantes (GET /v1/receipts, total 0).');
    } else {
      var ltEl = document.getElementById('last-task');
      ltEl.textContent = latestExec !== null ? '(#' + latestExec.n + ' · ' + (lastCycleTask || latestExec.task || '') + ')' : '(' + lastCycle.task + ')';
      var byGate = {};
      var lrows = lastCycle.recs.slice(0).sort(function(a, b){
        var ta = visionParseTime((a.receipt || {}).started_at);
        var tb2 = visionParseTime((b.receipt || {}).started_at);
        return (ta === null ? 0 : ta) - (tb2 === null ? 0 : tb2);
      });
      lrows.forEach(function(it){
        var rc = it.receipt || {};
        if(rc.gate && !byGate[rc.gate]){byGate[rc.gate] = rc.verdict || '';}
      });
      Object.keys(byGate).forEach(function(g){visionSetGateState(g, byGate[g]);});
      var shown = lrows.slice(0, 12);
      shown.forEach(function(it, idx){
        var rc = it.receipt || {};
        var tr = document.createElement('tr');
        tr.appendChild(tdNum(idx + 1));
        tr.appendChild(td(rc.gate || ''));
        var v = document.createElement('td');
        v.appendChild(verdictPill(rc.verdict));
        tr.appendChild(v);
        tr.appendChild(td(visionFmtDur(rc.duration_ms || 0)));
        tr.appendChild(tdClip(it.path || ''));
        tg.appendChild(tr);
      });
      if(lrows.length > shown.length){
        var mr = document.createElement('tr');
        var mc = document.createElement('td');mc.colSpan = 5;
        mc.textContent = '… ' + (lrows.length - shown.length) + ' compuertas más — ver Evidencia';
        mr.appendChild(mc);tg.appendChild(mr);
      }
    }
    setError('err-lastgates', null);
  }).catch(function(e){
    visionKpiUnavailable();
    setError('err-vision', e);
    setError('err-bars', e);
    setError('err-pass', e);
    setError('err-runs', e);
    setError('err-lastgates', e);
  });
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
    pageInfo('info-receipts', 'prev-receipts', 'next-receipts', d.total, d.limit, d.offset, 'GET /v1/receipts');
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
    var flt = [];
    if(q){flt.push('texto: ' + q);}
    if(t){flt.push('tipo: ' + t);}
    pageInfo('info-obs', 'prev-obs', 'next-obs', d.total, d.limit, d.offset, 'GET /v1/observations' + (flt.length ? ' — filtro: ' + flt.join(', ') : ' — sin filtro'));
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
      tr.appendChild(tdClip(txt));
      tb.appendChild(tr);
    });
    if((d.items || []).length === 0){emptyRow(tb, 3, 'Sin lecciones todavía: proyecto vacío o filtro sin resultados (GET /v1/lessons, total 0; filtro status=' + s + ').');}
    pageInfo('info-lessons', 'prev-lessons', 'next-lessons', d.total, d.limit, d.offset, 'GET /v1/lessons?status=' + s);
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
      tr.appendChild(tdNum(String(s.entries)));
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
    pageInfo('info-traces', 'prev-traces', 'next-traces', d.total, d.limit, d.offset, 'GET /v1/traces');
    setError('err-traces', null);
    var items = d.items || [];
    if(!state.session && items.length > 0){state.session = items[0].session;state.entries = 0;}
    if(state.session){loadEntries();}
    else{
      setText('trace-title', 'Pasos de la sesión — sin sesión (proyecto vacío o sin trazas)');
      var etb = document.getElementById('tb-entries');
      while(etb.firstChild){etb.removeChild(etb.firstChild);}
      emptyRow(etb, 3, 'Sin pasos: no hay sesión seleccionada (GET /v1/traces?session=..., total 0).');
      pageInfo('info-entries', 'prev-entries', 'next-entries', 0, PAGE, 0, 'GET /v1/traces?session= — sin sesión');
    }
  }).catch(function(e){setError('err-traces', e);});
}
function loadEntries(){
  if(!state.session){
    setText('trace-title', 'Pasos de la sesión — sin sesión (proyecto vacío o sin trazas)');
    var tb0 = document.getElementById('tb-entries');
    while(tb0.firstChild){tb0.removeChild(tb0.firstChild);}
    emptyRow(tb0, 3, 'Sin pasos: no hay sesión seleccionada (GET /v1/traces?session=..., total 0).');
    pageInfo('info-entries', 'prev-entries', 'next-entries', 0, PAGE, 0, 'GET /v1/traces?session= — sin sesión');
    return;
  }
  setText('trace-title', 'Pasos de la sesión ' + state.session);
  fetchData('/v1/traces?session=' + encodeURIComponent(state.session) + '&limit=' + PAGE + '&offset=' + state.entries).then(function(d){
    var tb = document.getElementById('tb-entries');
    while(tb.firstChild){tb.removeChild(tb.firstChild);}
    (d.items || []).forEach(function(e){
      var tr = document.createElement('tr');
      tr.appendChild(tdNum(e.call || ''));
      tr.appendChild(td(e.tool || ''));
      var msg = e.message || '';
      tr.appendChild(tdClip(msg));
      tb.appendChild(tr);
    });
    if((d.items || []).length === 0){emptyRow(tb, 3, 'Sin pasos en esta sesión (GET /v1/traces?session=' + state.session + ', total 0).');}
    pageInfo('info-entries', 'prev-entries', 'next-entries', d.total, d.limit, d.offset, 'GET /v1/traces?session=' + state.session);
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
    pageInfo('info-budget', 'prev-budget', 'next-budget', d.total, d.limit, d.offset, 'GET /v1/budget');
    setError('err-budget', null);
  }).catch(function(e){setError('err-budget', e);});
}
function stampUpdate(){
  var d = new Date();
  function p(n){return (n < 10 ? '0' : '') + n;}
  setText('last-update', 'Última actualización ' + p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds()));
}
function loadAll(){stampUpdate();loadHeader();loadCards();loadGraphs();loadVision();loadReceipts();loadObs();loadLessons();loadTraces();loadBudget();}
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
document.getElementById('project').addEventListener('change', function(){
  state.project = document.getElementById('project').value;
  state.receipts = 0;state.obs = 0;state.lessons = 0;state.traces = 0;state.entries = 0;state.budget = 0;state.session = '';
  loadAll();
});
document.getElementById('refresh').addEventListener('click', loadAll);
function loadProjects(){
  return fetchData('/v1/projects').then(function(list){
    var items = Array.isArray(list) ? list : [];
    var clr = document.getElementById('projects-error');
    if(clr){clr.textContent = '';}
    var sel = document.getElementById('project');
    var row = document.getElementById('project-row');
    while(sel.firstChild){sel.removeChild(sel.firstChild);}
    items.forEach(function(p){
      var o = document.createElement('option');
      o.value = p.name || '';
      o.textContent = p.name || '';
      sel.appendChild(o);
    });
    if(items.length > 1){
      if(row){row.hidden = false;}
      var ok = false;
      for(var i = 0; i < items.length; i++){if(items[i].name === state.project){ok = true;}}
      sel.value = ok ? state.project : (items[0].name || '');
    }else{
      if(row){row.hidden = true;}
      state.project = '';
    }
  }).catch(function(e){
    var m = document.getElementById('projects-error');
    if(m){m.textContent = 'No se pudo cargar la lista de proyectos (GET /v1/projects): ' + (e && e.message ? e.message : e);}
  });
}
loadProjects().then(loadAll);setInterval(loadAll,30000);
