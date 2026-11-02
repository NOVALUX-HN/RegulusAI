package httpapi

// V4 vision (charts/dashboard-vision.md): réplica pixel a pixel del mockup
// docs/assets/dashboard-vision.png — sidebar con 7 entradas con iconos
// (Resumen/Evidencia/Compuertas/Agentes/Métricas/Almanaque/Configuración),
// tarjeta de proyecto (nombre, rama de /v1/git, última ejecución), topbar
// con pill verde del último ciclo (#N de /v1/executions), 5 KPIs exactos
// del mockup con formato 1,248/98.8% y copy masticado, 2 gráficos SVG con
// ejes y banda, 2 tablas #/Rama y #/Compuerta con datos del backend.
// Agentes = sección real con la lista fija de guardians+crew de
// docs/agents.md. Configuración = ancla a la lista fija de las 12 puertas
// de regulus.json con estado vivo del último ciclo. Sin datos inventados:
// históricos sin manifest muestran rama "—"; si solo hay 1 checkout se
// avisa; deltas 7d calculados en cliente con cap 1000 ("parcial" si el
// total lo supera).
const dashboardHTML = `<!doctype html>
<html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
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
/* V1 mockup shell: skip link + flex app (sidebar + content). */
.skip-link{position:absolute;left:-9999px;top:0;background:var(--regulus-brand);color:var(--regulus-bg);padding:8px 16px;border-radius:0 0 var(--radius-md) 0;z-index:50;font-size:14px}
.skip-link:focus{left:0}
.app{display:flex;align-items:stretch}
.sidebar{width:268px;flex:0 0 268px;background:#0A0E14;border-right:var(--border-width) solid var(--regulus-brand-dark);padding:20px 16px;position:sticky;top:0;height:100vh;overflow-y:auto;display:flex;flex-direction:column;gap:20px}
.brand{display:flex;gap:12px;align-items:center;padding-bottom:16px;border-bottom:var(--border-width) solid var(--regulus-border)}
.brand-star{color:var(--regulus-brand);font-size:28px;line-height:1}
.brand strong{display:block;font-family:var(--font-heading);letter-spacing:.08em;color:var(--regulus-text);font-size:16px}
.brand small{color:var(--regulus-brand);letter-spacing:.18em;font-size:10px}
.side-nav ul{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:4px}
.side-nav a{display:block;padding:10px 14px;border-radius:var(--radius-md);color:var(--regulus-muted);text-decoration:none;font-size:14px;border:var(--border-width) solid transparent}
.side-nav a:hover{border-color:var(--regulus-brand);color:var(--regulus-text)}
.side-card{margin-top:auto;border:var(--border-width) solid var(--regulus-brand-dark);border-radius:var(--radius-lg);padding:16px;background:var(--regulus-surface);font-size:13px}
.side-card h2{font-size:12px;text-transform:uppercase;letter-spacing:.08em;color:var(--regulus-muted);margin:0 0 12px;border:0;padding:0}
.side-row{display:flex;justify-content:space-between;gap:8px;margin:8px 0;color:var(--regulus-muted)}
.side-row strong{color:var(--regulus-text);font-weight:600;text-align:right;overflow-wrap:anywhere}
.main-col{flex:1;min-width:0}
.topbar-row{display:flex;gap:16px;align-items:flex-start;justify-content:space-between;flex-wrap:wrap}
.pill{display:inline-flex;gap:8px;align-items:center;background:var(--regulus-surface);border:var(--border-width) solid var(--regulus-brand-dark);border-radius:var(--radius-xl);padding:8px 16px;font-size:14px;white-space:nowrap;max-width:100%}
.pill .dot{width:12px;height:12px;border-radius:50%;background:var(--status-inconclusive);flex:0 0 12px}
.pill.is-pass .dot{background:var(--status-pass)}
.pill.is-fail .dot{background:var(--status-fail)}
.pill span:last-child{overflow:hidden;text-overflow:ellipsis}
.kpi-grid{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:16px}
.kpi{border:var(--border-width) solid var(--regulus-brand-dark);border-radius:var(--radius-lg);padding:16px;background:var(--regulus-surface-raised);min-width:0}
.kpi h3{margin:0 0 8px;font-size:13px;color:var(--regulus-muted);font-weight:600}
.kpi .num{font-size:30px;font-weight:700;color:var(--regulus-brand-bright);font-family:var(--font-mono);font-variant-numeric:tabular-nums;overflow-wrap:anywhere}
.kpi .sub{font-size:12px;color:var(--regulus-muted);margin-top:4px}
.kpi .delta{font-size:12px;margin-top:8px;color:var(--regulus-muted)}
.delta-up{color:var(--status-pass);font-weight:600}
.delta-down{color:var(--status-fail);font-weight:600}
.panels{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}
.panel{border:var(--border-width) solid var(--regulus-brand-dark);border-radius:var(--radius-lg);padding:16px;background:var(--regulus-surface);min-width:0}
.panel h3{margin:0 0 8px;font-size:16px}
.panel .src{font-size:12px;color:var(--regulus-muted);margin:0 0 12px}
svg.chart{width:100%;height:220px;display:block}
svg.chart text{fill:var(--regulus-muted);font-size:10px;font-family:var(--font-body)}
.grid-line{stroke:var(--regulus-border);stroke-width:1}
.axis-line{stroke:var(--regulus-muted);stroke-width:1}
.tbl-wide{overflow-x:auto;max-width:100%;-webkit-overflow-scrolling:touch}
.tbl-wide thead th{position:sticky;top:0;z-index:1}
.tbl-wide td{word-break:break-word}
#vt-runs th,#vt-lastgates th{white-space:nowrap;font-size:11px}
#vt-runs th,#vt-runs td,#vt-lastgates th,#vt-lastgates td{padding:6px}
#tb-runs td,#tb-lastgates td{vertical-align:middle}
#tb-runs td{font-size:12px}
#tb-lastgates td{font-size:13px}
#tb-runs td:nth-child(1),#tb-runs td:nth-child(2),#tb-runs td:nth-child(3),#tb-runs td:nth-child(4),#tb-runs td:nth-child(5),#tb-runs td:nth-child(6),
#tb-lastgates td:nth-child(1),#tb-lastgates td:nth-child(2),#tb-lastgates td:nth-child(3),#tb-lastgates td:nth-child(4){white-space:nowrap}
.more-link{margin-top:12px;display:inline-block}
.vision-note{font-size:12px;color:var(--regulus-muted)}
.side-nav a svg{flex:0 0 16px}
.icon{color:var(--regulus-brand);display:inline-flex;line-height:0}
.pill.is-pass{border-color:var(--status-pass)}
.pill.is-fail{border-color:var(--status-fail)}
.kpi .what{font-size:12px;color:var(--regulus-text);margin:8px 0 0}
.agent-list,.gate-list{list-style:none;margin:0;padding:0;display:grid;gap:8px;font-size:14px}
.agent-list li,.gate-list li{display:flex;gap:10px;align-items:baseline;justify-content:space-between;border-bottom:var(--border-width) solid var(--regulus-border);padding:6px 0}
.agent-list .who{font-family:var(--font-mono);font-size:13px;color:var(--regulus-brand-bright)}
.gate-list .st{font-size:12px}
header.top{background:var(--regulus-bg);color:var(--regulus-text);padding:24px 24px;border-bottom:2px solid var(--regulus-brand)}
header.top .wrap,main{max-width:1100px;margin:0 auto}
header.top h1{margin:0 0 8px;font-size:28px;font-family:var(--font-heading);color:var(--regulus-brand-bright);letter-spacing:.01em}
header.top p{margin:8px 0;color:var(--regulus-muted)}
#status,#health{margin-top:16px;display:inline-block;background:var(--regulus-surface);border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-md);padding:8px 16px;font-size:14px;margin-right:8px;color:var(--regulus-text)}
main{padding:24px}
section{background:var(--regulus-surface);border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-lg);padding:24px;margin:0 0 24px}
h2,h3{font-family:var(--font-heading)}
section>h2{margin:0 0 8px;font-size:20px;color:var(--regulus-text);border-left:4px solid var(--regulus-brand);padding-left:12px;line-height:1.4}
#graphs h3{font-size:13px;text-transform:uppercase;letter-spacing:.06em;color:var(--regulus-muted);margin:16px 0 8px;font-family:var(--font-body);font-weight:600}
#graphs h3:first-child{margin-top:8px}
#trace-title{margin:24px 0 8px;padding-top:16px;border-top:var(--border-width) solid var(--regulus-border);font-size:16px}
p.plain{color:var(--regulus-muted);margin:8px 0 16px;font-size:14px}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:16px}
.card{border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-md);padding:16px;background:var(--regulus-surface-raised)}
.card h3{margin:0 0 8px;font-size:15px;color:var(--regulus-text)}
.card .num{font-size:24px;font-weight:700;color:var(--regulus-brand-bright);font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.card p{margin:8px 0 0;font-size:13px;color:var(--regulus-muted)}
.bar-row{display:grid;grid-template-columns:180px 1fr 60px;gap:8px;align-items:center;margin:8px 0;font-size:13px}
.bar-track{background:var(--regulus-bg);border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-sm);height:14px;overflow:hidden}
.bar-fill{height:100%;background:var(--module-sextant);border-radius:var(--radius-sm);transition:width var(--transition-normal)}
.bar-fill.fill-pass{background:var(--status-pass)}
.bar-fill.fill-fail{background:var(--status-fail)}
.bar-fill.fill-blocked{background:var(--status-blocked)}
.bar-fill.fill-inconclusive{background:var(--status-inconclusive)}
.bar-row .v{text-align:right;font-variant-numeric:tabular-nums;font-family:var(--font-mono)}
table{width:100%;border-collapse:collapse;font-size:14px}
th,td{text-align:left;padding:8px;border-bottom:var(--border-width) solid var(--regulus-border);vertical-align:top}
th{background:var(--regulus-surface-raised);font-size:12px;text-transform:uppercase;letter-spacing:.04em;color:var(--regulus-muted);font-family:var(--font-body)}
th.num,td.num{text-align:right}
td.num,.t-mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.pager{display:flex;gap:8px;align-items:center;margin-top:16px;font-size:13px;color:var(--regulus-muted)}
button{border:var(--border-width) solid var(--regulus-border);background:var(--regulus-surface-raised);color:var(--regulus-text);border-radius:var(--radius-md);padding:8px 16px;cursor:pointer;font-size:14px;font-family:var(--font-body);transition:background var(--transition-fast),border-color var(--transition-fast)}
button:hover{border-color:var(--regulus-brand)}
button:disabled{opacity:.45;cursor:default}
button.primary{background:var(--regulus-brand);color:var(--regulus-bg);border-color:var(--regulus-brand)}
.controls{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:16px}
input,select{border:var(--border-width) solid var(--regulus-border);border-radius:var(--radius-md);padding:8px 16px;font-size:14px;background:var(--regulus-surface-raised);color:var(--regulus-text);font-family:var(--font-body)}
.proj-row{display:flex;gap:8px;align-items:center;margin-top:16px}
.proj-row label{font-size:14px;color:var(--regulus-muted)}
#project{min-width:200px}
.verdict-pass,.ok{color:var(--status-pass);font-weight:600}
.verdict-fail,.fail{color:var(--status-fail);font-weight:600}
.verdict-blocked{color:var(--status-blocked);font-weight:600}
.verdict-inconclusive{color:var(--status-inconclusive);font-weight:600}
.verdict-pass,.verdict-fail,.verdict-blocked,.verdict-inconclusive,.ok,.fail{display:inline-block;border:var(--border-width) solid currentColor;border-radius:var(--radius-xl);padding:1px 10px;font-size:13px}
.err{color:var(--status-fail);font-size:13px}
footer{color:var(--regulus-muted);font-size:13px;text-align:center;padding:0 0 24px}
details{margin-top:8px;font-size:13px}
summary{cursor:pointer}
pre{background:var(--regulus-bg);color:var(--regulus-text);border:var(--border-width) solid var(--regulus-border);padding:16px;border-radius:var(--radius-md);overflow:auto;font-size:12px;font-family:var(--font-mono)}
a{color:var(--regulus-brand-bright)}
:focus-visible{outline:2px solid var(--regulus-brand-bright);outline-offset:2px;border-radius:var(--radius-sm)}
@media (prefers-reduced-motion:reduce){
  *{transition:none;animation:none}
  .bar-fill{transition:none}
}
@media (max-width:640px){
  .bar-row{grid-template-columns:1fr;gap:8px}
  .bar-row .v{text-align:left}
  header.top{padding:16px 16px}
  main{padding:16px}
  #cards-grid{grid-template-columns:1fr}
  .pager{flex-wrap:wrap}
  .proj-row{flex-wrap:wrap}
  #project{flex:1 1 200px;min-width:0}
  button,input,select{min-height:44px;font-size:16px}
  .app{flex-direction:column}
  .sidebar{position:static;width:auto;flex:none;height:auto;overflow:visible;border-right:0;border-bottom:var(--border-width) solid var(--regulus-brand-dark)}
  .side-nav ul{flex-direction:row;overflow-x:auto}
  .side-nav a{white-space:nowrap}
  .side-card{margin-top:0}
  .kpi-grid{grid-template-columns:1fr}
  .panels{grid-template-columns:1fr}
  .topbar-row{flex-direction:column}
}
@media (pointer:coarse){
  button,input,select{min-height:44px}
}
/* R1: tablas consultables a 360px sin scroll horizontal de página.
   Técnica elegida (las 6 tablas): cada tabla
   vive en un .tbl-wrap con scroll horizontal propio, porque el apilado
   por filas destruiría la comparación entre filas (recibos, sesiones,
   presupuesto). El Then de R1 admite "apilado o scroll contenido". */
html,body{max-width:100vw;overflow-x:hidden}
main,header.top .wrap,section{max-width:100%;min-width:0}
.tbl-wrap{overflow-x:auto;max-width:100%;-webkit-overflow-scrolling:touch}
.tbl-wrap thead th{position:sticky;top:0;z-index:1}
th,td{overflow-wrap:anywhere}
.tbl-wrap td{word-break:break-word}
</style>
</head><body>
<a class="skip-link" href="#main">Saltar al contenido</a>
<!-- V4 réplica del mockup docs/assets/dashboard-vision.png: sidebar con
  las 7 entradas (Agentes y Configuración llevan a secciones reales más
  abajo); ramas e #N salen de /v1/executions+/v1/git (históricos sin
  manifest = "—"); deltas 7d en cliente con cap 1000 por colección
  ("parcial" si el total lo supera). Nada hardcodeado del mockup. -->
<div class="app">
<nav class="sidebar" aria-label="Secciones del panel">
<div class="brand"><span class="brand-star" aria-hidden="true">✦</span><div><strong>REGULUS AI</strong><small>THE LION STAR HARNESS</small></div></div>
<div class="side-nav"><ul>
<li><a href="#sec-resumen"><span class="icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 7.5 8 2.5l6 5V13.5H9.5V10h-3v3.5H2z"/></svg></span>Resumen</a></li>
<li><a href="#sec-evidencia"><span class="icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M8 1.5 14 4.5 8 7.5 2 4.5z"/><path d="M2 8l6 3 6-3"/><path d="M2 11.5l6 3 6-3"/></svg></span>Evidencia</a></li>
<li><a href="#sec-compuertas"><span class="icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M8 1.5 13 3.5v4c0 3.5-2.5 5.7-5 7-2.5-1.3-5-3.5-5-7v-4z"/><path d="M5.8 7.8l1.6 1.6 2.8-3"/></svg></span>Compuertas</a></li>
<li><a href="#sec-agentes"><span class="icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="5.5" cy="5" r="2.3"/><path d="M1.5 13.5c.5-2.3 2-3.5 4-3.5s3.5 1.2 4 3.5"/><circle cx="11.5" cy="6" r="1.8"/><path d="M11 10.2c1.7.2 3 1.3 3.5 3"/></svg></span>Agentes</a></li>
<li><a href="#sec-metricas"><span class="icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 13.5 6 8.5l2.5 2.5L14 3.5"/><path d="M10.5 3.5H14V7"/></svg></span>Métricas</a></li>
<li><a href="#sec-memoria"><span class="icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 2h7.5c1.8 0 2.5.8 2.5.8v11s-.7-.8-2.5-.8H3z"/><path d="M3 2v11"/><path d="M6 5.5h4"/></svg></span>Almanaque</a></li>
<li><a href="#sec-config"><span class="icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="2.2"/><path d="M8 1.5v2.2M8 12.3v2.2M1.5 8h2.2M12.3 8h2.2M3.4 3.4l1.6 1.6M11 11l1.6 1.6M12.6 3.4 11 5M5 11l-1.6 1.6"/></svg></span>Configuración</a></li>
</ul></div>
<div class="side-card" aria-label="Proyecto"><h2>Proyecto</h2>
<div class="side-row"><span>Proyecto</span><strong id="proj-name">cargando…</strong></div>
<div class="side-row"><span>Rama</span><strong id="proj-branch">cargando…</strong></div>
<div class="side-row"><span>Última ejecución</span><strong id="proj-activity">cargando…</strong></div>
</div>
</nav>
<div class="main-col">
<header class="top"><div class="wrap">
<div class="topbar-row">
<div><h1 id="project-name">Regulus — Qué hay guardado en tu proyecto</h1>
<p>Evidencia, compuertas y aprendizaje de cada ciclo del harness. Esta página solo mira: nunca cambia ni borra nada.</p></div>
<div class="pill" id="last-cycle" role="status"><span class="dot" aria-hidden="true"></span><span id="last-cycle-text">Último ciclo: cargando…</span></div>
</div>
<div class="proj-row" id="project-row" hidden><label for="project">Proyecto</label><select id="project" aria-label="Proyecto"></select></div>
<p class="err" id="projects-error"></p>
<div id="status">Estado del proyecto: cargando…</div>
<div id="health">Salud del sistema: cargando…</div>
<details><summary>Ver estado completo del proyecto</summary><pre id="status-full">cargando…</pre></details>
</div></header>
<main id="main">
<section id="sec-resumen"><h2>Resumen</h2>
<p class="plain">Las cinco cifras del proyecto, como en el mockup: cada una dice qué es en palabras sencillas y de dónde sale. Calculadas en tu navegador desde los endpoints de solo lectura (sin inventar nada).</p>
<div class="kpi-grid">
<article class="kpi"><h3>Evidencias</h3><div class="num" id="kpi-evidencias-val">…</div><div class="delta" id="kpi-evidencias-delta">cargando…</div><p class="what">Qué significa: comprobantes guardados (uno por cada prueba pasada por el harness).</p><p class="sub">Fuente: GET /v1/metrics campo receipts_total (global); delta % 7 días desde started_at.</p></article>
<article class="kpi"><h3>Ejecuciones</h3><div class="num" id="kpi-ejecuciones-val">…</div><div class="delta" id="kpi-ejecuciones-delta">cargando…</div><p class="what">Qué significa: ciclos numerados del harness (#1 el más antiguo).</p><p class="sub">Fuente: GET /v1/executions campo total; delta % 7 días desde date.</p></article>
<article class="kpi"><h3>Compuertas (PASS)</h3><div class="num" id="kpi-pass-val">…</div><div class="sub" id="kpi-pass-sub">cargando…</div><div class="delta" id="kpi-pass-delta">cargando…</div><p class="what">Qué significa: de cada 100 pruebas, cuántas salieron bien.</p><p class="sub">Fuente: GET /v1/metrics campo verification_pass_rate + receipts_pass/receipts_total.</p></article>
<article class="kpi"><h3>Lecciones</h3><div class="num" id="kpi-lecciones-val">…</div><div class="delta" id="kpi-lecciones-delta">cargando…</div><p class="what">Qué significa: cosas que Regulus ya aprendió y no olvida.</p><p class="sub">Fuente: GET /v1/almanac/summary campo lessons_admitted; delta 7 días desde CreatedAt.</p></article>
<article class="kpi"><h3>Ramas activas</h3><div class="num" id="kpi-ramas-val">…</div><div class="delta" id="kpi-ramas-delta">cargando…</div><p class="what">Qué significa: ramas vistas en los ciclos más la actual.</p><p class="sub">Fuente: GET /v1/executions campo branch + GET /v1/git (rama actual).</p><p class="sub" id="kpi-ramas-actual">cargando…</p></article>
</div>
<p class="plain vision-note" id="vision-partial" hidden></p>
<p class="err" id="err-vision"></p></section>
<section id="sec-metricas"><h2>Métricas</h2>
<p class="plain">Actividad diaria de los últimos 7 días y tasa de éxito diaria, agrupando los comprobantes por su fecha de inicio. Sin barras ni puntos cuando un día no tiene datos.</p>
<div class="panels">
<div class="panel"><h3>Ejecuciones del harness (últimos 7 días)</h3><p class="plain">Qué significa: cuántos comprobantes salieron cada día (más barras = más trabajo verificado).</p><p class="src">Fuente: GET /v1/receipts paginado (cap 1000; agrupa started_at por día).</p><svg class="chart" id="chart-bars" role="img" aria-label="Barras de ejecuciones por día"><title id="chart-bars-title">Ejecuciones por día</title></svg><p class="err" id="err-bars"></p></div>
<div class="panel"><h3>Tasa de éxito de compuertas</h3><p class="plain">Qué significa: qué parte de las pruebas salió bien cada día (arriba = mejor; la banda marca la zona).</p><p class="src">Fuente: GET /v1/receipts paginado (cap 1000; pass/total por día).</p><svg class="chart" id="chart-pass" role="img" aria-label="Línea de tasa de éxito por día"><title id="chart-pass-title">Tasa de éxito por día</title></svg><p class="err" id="err-pass"></p></div>
</div></section>
<section id="sec-compuertas"><h2>Compuertas</h2>
<p class="plain">Cada fila es un ciclo numerado del harness (#1 el más antiguo): su rama, si salió bien, cuántas pruebas pasaron, cuánto tardó y cuándo fue. La segunda tabla abre el último ciclo prueba por prueba.</p>
<div class="panels">
<div class="panel"><h3>Últimas ejecuciones</h3><p class="src">Fuente: GET /v1/executions (ciclos #N con rama del manifest; sin manifest = —; las más nuevas primero).</p><div class="tbl-wide"><table id="vt-runs"><thead><tr><th scope="col">#</th><th scope="col">Rama</th><th scope="col">Resultado</th><th scope="col">Compuertas</th><th scope="col">Duración</th><th scope="col">Fecha</th></tr></thead><tbody id="tb-runs"></tbody></table></div><a class="more-link" href="#sec-evidencia">Ver todas →</a><p class="err" id="err-runs"></p></div>
<div class="panel"><h3>Compuertas del último ciclo <span id="last-task"></span></h3><p class="src">Fuente: GET /v1/receipts agrupados como /v1/executions (mismo run_id; la ruta es la evidencia de cada prueba).</p><div class="tbl-wide"><table id="vt-lastgates"><thead><tr><th scope="col">#</th><th scope="col">Compuerta</th><th scope="col">Estado</th><th scope="col">Tiempo</th><th scope="col">Evidencia</th></tr></thead><tbody id="tb-lastgates"></tbody></table></div><a class="more-link" href="#sec-evidencia">Ver todas →</a><p class="err" id="err-lastgates"></p></div>
</div></section>
<section id="cards"><h2>Resumen por áreas</h2>
<p class="plain">Cada tarjeta cuenta cuántas cosas hay guardadas en una parte del proyecto y qué significa.</p>
<div class="grid" id="cards-grid"></div>
<p class="plain" id="cards-empty" hidden></p>
<p class="err" id="cards-error"></p></section>
<section id="sec-graphs"><h2>De un vistazo</h2>
<p class="plain">Mini gráficos con los repartos más útiles: qué tipos de recuerdos hay, lecciones confirmadas frente a pendientes, en qué se usó el presupuesto y cómo salieron las comprobaciones.</p>
<div id="graphs"></div>
<p class="plain">Leyenda: PASS = comprobación superada · FAIL = falló · BLOCKED = bloqueada o por confirmar · Confirmada = lección admitida · Por confirmar = candidata · Sin dato = sin veredicto registrado.</p>
<p class="err" id="graphs-error"></p></section>
<section id="sec-evidencia"><h2>Evidencia — comprobantes de verificación</h2>
<p class="plain">Cada fila es una vez que Regulus comprobó que el trabajo estaba bien. Es el ticket de la prueba: qué tarea, qué prueba y si salió bien. Fuente: GET /v1/receipts, 10 por página; el paginador indica la página y el total.</p>
<div class="tbl-wrap"><table><thead><tr><th>Tarea</th><th>Prueba</th><th>Resultado</th><th>Fecha</th></tr></thead><tbody id="tb-receipts"></tbody></table></div>
<div class="pager"><button id="prev-receipts">Anterior</button><span id="info-receipts"></span><button id="next-receipts">Siguiente</button></div>
<p class="err" id="err-receipts"></p></section>
<section id="sec-memoria"><h2>Memoria — notas y decisiones</h2>
<p class="plain">Lo que Regulus anotó para no olvidarlo: notas de trabajo, decisiones y descubrimientos. Usa el buscador para encontrar algo concreto. Fuente: GET /v1/observations con filtros de texto y tipo; el paginador muestra el filtro activo.</p>
<div class="controls"><input id="q-obs" type="search" placeholder="Buscar en memoria…" aria-label="Buscar en memoria"><select id="type-obs" aria-label="Tipo de recuerdo"><option value="">Todos los tipos</option><option value="session-note">Notas</option><option value="decision">Decisiones</option><option value="discovery">Descubrimientos</option><option value="lesson">Lecciones</option><option value="preference">Preferencias</option></select><button id="go-obs" class="primary">Buscar</button></div>
<div class="tbl-wrap"><table><thead><tr><th>Título</th><th>Tipo</th><th>Fecha</th></tr></thead><tbody id="tb-obs"></tbody></table></div>
<div class="pager"><button id="prev-obs">Anterior</button><span id="info-obs"></span><button id="next-obs">Siguiente</button></div>
<p class="err" id="err-obs"></p></section>
<section id="sec-aprendizaje"><h2>Aprendizaje — lecciones</h2>
<p class="plain">Lo que Regulus ya aprendió (confirmado) y lo que cree haber aprendido pero aún está por confirmar (candidato). Fuente: GET /v1/lessons con filtro de estado; el paginador muestra el filtro activo.</p>
<div class="controls"><select id="status-lessons" aria-label="Estado de la lección"><option value="all">Confirmadas + por confirmar</option><option value="admitted">Solo confirmadas</option><option value="candidate">Solo por confirmar</option></select><button id="go-lessons" class="primary">Filtrar</button></div>
<div class="tbl-wrap"><table><thead><tr><th>Tarea</th><th>Estado</th><th>Lección</th></tr></thead><tbody id="tb-lessons"></tbody></table></div>
<div class="pager"><button id="prev-lessons">Anterior</button><span id="info-lessons"></span><button id="next-lessons">Siguiente</button></div>
<p class="err" id="err-lessons"></p></section>
<section id="sec-sesiones"><h2>Sesiones — el trabajo paso a paso</h2>
<p class="plain">Cada sesión es una racha de trabajo seguida. Pulsa «Ver pasos» para leer qué hizo Regulus en cada paso, en orden. Fuentes: GET /v1/traces (sesiones) y GET /v1/traces?session= (pasos), 10 por página.</p>
<div class="tbl-wrap"><table><thead><tr><th>Sesión</th><th class="num">Número de pasos</th><th>Acción</th></tr></thead><tbody id="tb-traces"></tbody></table></div>
<div class="pager"><button id="prev-traces">Anterior</button><span id="info-traces"></span><button id="next-traces">Siguiente</button></div>
<h3 id="trace-title">Pasos de la sesión</h3>
<div class="tbl-wrap"><table><thead><tr><th class="num">Paso</th><th>Herramienta</th><th>Qué hizo</th></tr></thead><tbody id="tb-entries"></tbody></table></div>
<div class="pager"><button id="prev-entries">Anterior</button><span id="info-entries"></span><button id="next-entries">Siguiente</button></div>
<p class="err" id="err-traces"></p></section>
<section id="sec-agentes"><h2>Agentes — quién hace qué</h2>
<p class="plain">Qué significa: la gente (de mentira) que trabaja en tu proyecto: los guardianes solo miran y avisan, la tripulación sí toca el código. Lista fija del repo (docs/agents.md), no inventada por el panel.</p>
<div class="panels">
<div class="panel"><h3>Guardianes (solo miran, no editan)</h3><p class="src">Fuente: docs/agents.md (lista fija del repo).</p><ul class="agent-list">
<li><span class="who">@regulus-gate-verifier</span><span>Prueba que cada tarea está hecha (gauntlet + evidencias).</span></li>
<li><span class="who">@regulus-spec-keeper</span><span>Juzga si los criterios de aceptación son observables.</span></li>
<li><span class="who">@regulus-mutation-auditor</span><span>Rompe el código a propósito para ver si los tests lo notan.</span></li>
<li><span class="who">@regulus-crap-analyst</span><span>Señala el código enredado y cómo desenredarlo.</span></li>
<li><span class="who">@regulus-evidence-auditor</span><span>Comprueba que cada «hecho» cite una evidencia que existe.</span></li>
<li><span class="who">@regulus-reviewer</span><span>Lee el código cambiado y dice qué sobra.</span></li>
</ul></div>
<div class="panel"><h3>Tripulación (sí edita, una tarea acotada)</h3><p class="src">Fuente: docs/agents.md (lista fija del repo).</p><ul class="agent-list">
<li><span class="who">@regulus-scout</span><span>Explora y dibuja el mapa antes de tocar nada.</span></li>
<li><span class="who">@regulus-worker</span><span>Construye una unidad pequeña y la devuelve probada.</span></li>
<li><span class="who">@regulus-verifier</span><span>Reproduce el fallo y comprueba que el arreglo lo quita.</span></li>
<li><span class="who">@regulus-proposer</span><span>Propone mejoras del propio sistema con evidencias.</span></li>
</ul></div>
</div></section>
<section id="sec-config"><h2>Configuración — las 12 puertas del gauntlet</h2>
<p class="plain">Qué significa: la lista de pruebas que cada ciclo debe pasar para salir PASS. La lista es fija (sale de regulus.json); el estado de cada puerta es vivo y sale del último ciclo (tabla «Compuertas del último ciclo»).</p>
<div class="panel"><h3>Puertas y estado en el último ciclo</h3><p class="src">Fuente: lista fija de regulus.json (las 12 puertas); estado de GET /v1/receipts del último ciclo (— = esa puerta no corrió en este ciclo).</p><ul class="gate-list" id="gate-list">
<li><span>typecheck</span><span class="st" id="cfg-typecheck">cargando…</span></li>
<li><span>lint</span><span class="st" id="cfg-lint">cargando…</span></li>
<li><span>build</span><span class="st" id="cfg-build">cargando…</span></li>
<li><span>unit-tests</span><span class="st" id="cfg-unit-tests">cargando…</span></li>
<li><span>coverage-floor</span><span class="st" id="cfg-coverage-floor">cargando…</span></li>
<li><span>mutation</span><span class="st" id="cfg-mutation">cargando…</span></li>
<li><span>crap</span><span class="st" id="cfg-crap">cargando…</span></li>
<li><span>vulncheck</span><span class="st" id="cfg-vulncheck">cargando…</span></li>
<li><span>spec</span><span class="st" id="cfg-spec">cargando…</span></li>
<li><span>ts-typecheck</span><span class="st" id="cfg-ts-typecheck">cargando…</span></li>
<li><span>ts-unit-tests</span><span class="st" id="cfg-ts-unit-tests">cargando…</span></li>
<li><span>gofmt</span><span class="st" id="cfg-gofmt">cargando…</span></li>
</ul></div></section>
<section id="sec-coste"><h2>Coste — presupuesto de mejora</h2>
<p class="plain">Cada fila es un movimiento del presupuesto que Regulus usa para mejorar solo: qué día, de qué tipo, en qué tarea y cómo terminó. Fuente: GET /v1/budget, 10 por página.</p>
<div class="tbl-wrap"><table><thead><tr><th>Fecha</th><th>Tipo</th><th>Tarea</th><th>Resultado</th></tr></thead><tbody id="tb-budget"></tbody></table></div>
<div class="pager"><button id="prev-budget">Anterior</button><span id="info-budget"></span><button id="next-budget">Siguiente</button></div>
<p class="err" id="err-budget"></p></section>
</main>
</div>
</div>
<footer>Se actualiza solo cada 30 segundos. <span id="last-update">Última actualización --:--:--</span> <button id="refresh">Actualizar ahora</button></footer>
<script>
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
</script></body></html>`
