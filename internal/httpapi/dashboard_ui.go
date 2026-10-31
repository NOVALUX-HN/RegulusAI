package httpapi

// dashboardHTML is the always-on visual (T2, S1-S2): three views over the
// read-only /v1/* JSON — Harness, Guardado, Aprendizaje — with 30s
// auto-refresh. No dependencies, no writes, stdlib fetch only.
const dashboardHTML = `<!doctype html>
<html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Regulus — Harness vivo</title>
<style>body{font-family:system-ui;margin:24px;max-width:960px}section{border:1px solid #ddd;border-radius:8px;padding:16px;margin:16px 0}pre{background:#f6f6f6;padding:12px;border-radius:6px;overflow:auto}.ok{color:#0a0}.tabs button{margin-right:8px}</style>
</head><body>
<h1>Regulus — Harness vivo</h1>
<div class="tabs"><button onclick="show('harness')">Harness</button><button onclick="show('saved')">Guardado</button><button onclick="show('learn')">Aprendizaje</button></div>
<section id="harness"><h2>Harness</h2><pre id="p-harness">cargando…</pre></section>
<section id="saved" style="display:none"><h2>Guardado (Almanac)</h2><pre id="p-saved">cargando…</pre></section>
<section id="learn" style="display:none"><h2>Aprendizaje</h2><pre id="p-learn">cargando…</pre></section>
<script>
function show(id){for(const s of ['harness','saved','learn'])document.getElementById(s).style.display=s===id?'':'none'}
async function load(){try{
const [st,me,al]=await Promise.all([fetch('/v1/status').then(r=>r.json()),fetch('/v1/metrics').then(r=>r.json()),fetch('/v1/almanac/summary').then(r=>r.json())]);
document.getElementById('p-harness').textContent=JSON.stringify({status:st.data,metrics:me.data},null,2);
document.getElementById('p-saved').textContent=JSON.stringify(al.data,null,2);
const m=me.data&&me.data.metrics||{};document.getElementById('p-learn').textContent=JSON.stringify({evolution_acceptance:m.evolution_acceptance_rate,guardian_disagreement:m.guardian_disagreement_rate,experiments:{total:m.experiments_total,adopted:m.experiments_adopted},coverage:m.coverage},null,2);
}catch(e){document.getElementById('p-harness').textContent='error: '+e}}
load();setInterval(load,30000);
</script></body></html>`
