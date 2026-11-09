# fresh-fixes — segunda barrida del harness (10 gaps nuevos)

Header: 10 gaps frescos del scan post-speed-tokens, cada uno con RED/GREEN observado. Branch: develop (RDD afuera por decisión del dueño; merge a main solo cuando lo pida).
Read until `## Log` before edits.

## Specs

- S1: "olvidate de todos los pasos de RDD. analiza todo nuevamente e identifica otros puntos de mejora"
- S2: "ejecuta y actualiza el todo con esta lista nueva"
- S3: Reglas por item: F1 sin CI local (verificación estructural yaml); F3 con passthrough session/charts (no solo-docs); F6 medir `time regulus status` antes de dimensionar; F10 mantiene exit 2 y actualiza los tests que pinean el help; cada cambio con test nuevo o actualizado que pruebe el fix; suite verde al cierre de cada unidad; commits work-unit en develop.

## Tasks

- [x] F1 (S2,S3) — CI en develop: `branches: [main, develop]` en push + PR — route: inline — commit: d98b6a3
- [x] F2 (S2,S3) — plugin memory_search sin terms requerido — route: inline (small enough) — commit: 2dcde1a
- [x] F3 (S2,S3) — memory_context manual con lessons (params session/charts en plugin+MCP) — route: worker mw8phu9d-d-8e7g — commit: 393587c
- [ ] F4 (S2,S3) — gate_run tools con quick/profile/dry_run — route: worker — commit: —
- [x] F5 (S2,S3) — serve acepta forma con espacio — route: inline — commit: 48108c2
- [ ] F6 (S2,S3) — status sin full-scan (medir, cap 200 newest, memo gremlins) — route: worker — commit: —
- [x] F7 (S2,S3) — podar muertos: --no-staleness en context + root en validateTarget — route: inline — commit: b8918a9
- [x] F8 (S2,S3) — docs drift: server.go rutas + audit 19/14 (con pin) — route: inline — commit: 807a9f7
- [ ] F9 (S2,S3) — caveat del plugin a stderr en failure — route: inline — commit: —
- [ ] F10 (S2,S3) — did-you-mean en comando desconocido (exit 2 intacto) — route: inline — commit: —

## Log

- L1: "olvidate de todos los pasos de RDD. analiza todo nuevamente e identifica otros puntos de mejora"
- L2: "ejecuta y actualiza el todo con esta lista nueva"
- L3: scan mw8ok4aw-c-4xsu + spot-checks del parent (gauntlet.yml, required terms, context pelado, gate_run task-only, serve =-only) — 10 items verificados como reales antes de implementar.
- L4: F1 done — ruby -ryaml parse OK + ambas listas pineadas [main, develop]; sin push-back en el workflow (S8 retirado, solo artifacts), seguro en develop.
- L5: F2 done 2dcde1a — RED scrape (required terms) -> GREEN required []; bun lib 166/166, tsc limpio.
- L6: F5 done — RED 4 casos tabla -> GREEN; forma-espacio normalizada a =-form (validación única); cmd suite verde, vet/fmt limpios, binario vivo (8080 arranca, errores exit 2).
- L9: F8 done — server.go lista 13 rutas + dashboard; audit con notas F8 fechadas (original intacto, 19/14); pin en código ya existía (TestMCPIntentionalGapPinsPluginDelta PASS, sin test nuevo).
- L8: F7 done — test T3 invertido a refusal (RED exit 0->want 2) -> GREEN; --no-staleness en context rehúsa exit 2 (doctrina R3-001), usage recortado, root dropeado de validateTarget, fila docs/memory.md corregida; cli+surface verdes, vet/fmt limpios.
- L7: F3 done — RED 4 tests -> GREEN; session/charts opcionales en ambos tools, bare byte-identical; Go focused 2/2 + suite ok, bun 168/168, tsc limpio, vet/fmt limpios, diff revisado.
