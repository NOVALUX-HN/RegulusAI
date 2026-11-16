# merge-ready — poner CI verde + release mechanics antes del merge a main

Header: CI rojo en develop bloquea el merge (CRAP + perf marginal). Branch: develop.
Read until `## Log` before edits.

## Specs

- S1: "que mas nos falta resolver antes de hacer merge a main?"
- S2: "procede"
- S3: C1 paga CRAP <30 en las 4 funciones que nuestro trabajo subió (mcpTools, serve, initCmd, resolveDomainInProcess) partiendo + testeando, sin bajar el floor ni tocar el gate; C2 dictamina el perf (rerun primero: si pasa es flake; si repite es regresión y se investiga); C3 (bump/CHANGELOG/tag) espera decisión de número del dueño. Cada unidad con RED/GREEN y suite verde; commits work-unit en develop.

## Tasks

- [x] C1 (S2,S3) — CRAP paydown mcpTools/serve/initCmd/resolveDomainInProcess — route: mixed — commit: 13f39f8,791c8c2,420a21d,88aec88
- [x] C2 (S2,S3) — perf rerun + veredicto flake/regresión — route: inline — commit: done (verificación, ver L9)
- [x] C3 (S2,S3) — release 0.13.0 (bump/CHANGELOG/chart, tag en main pendiente) — route: inline — commit: 5742955

## Log

- L1: "que mas nos falta resolver antes de hacer merge a main?"
- L2: "procede"
- L7: C3 done 0.13.0 — package.json/versioninfo/manifest/README/CHANGELOG + chart release-0-13-0; rebuild stampleado (6 binarios + local) y doctor matches; suite doctor/cli verde. Tag v0.13.0 en main pendiente del dueño (igual que el merge).
- L6: C1 initCmd done 88aec88 — split en 8 helpers, cx 25->11, CRAP 32.79->11.51, mensajes/exits byte-identical; 14/14 init tests intactos, suite verde.
- L5: C1 serve+resolveDomain done (commits 791c8c2, 420a21d) — serve cx 7->2 (CRAP 39.22->3.19), resolve cx 25->9 (CRAP 31.61->9.03); suites verdes.
- L4: C1 mcpTools done 13f39f8 — split en 5 grupos, orden/schemas byte-identical (pin 17 en orden PASS), CRAP 44.03->fuera de lista (max 28.69 yolo).
- L3: evidencia CI (artifacts ci__push): run 42560200062 cae en unit-tests solo por TestPerfHybridP95Budget10k (p95 261 vs 250, resto verde, coverage 86.8%); run 42560036217 pasa unit-tests y cae en crap (mcpTools 44.03, serve 39.22, initCmd 32.79, resolveDomainInProcess 31.61; yolo 28.69 pasa).
- L8 (2026-11-16, sync done): C1 done. Evidencia: mcpTools 13f39f8 (CRAP 44.03->fuera de lista), serve 791c8c2 (39.22->3.19), resolveDomain 420a21d (31.61->9.03), initCmd 88aec88 (32.79->11.51). Mensajes/exits byte-identical, suites verdes.
- L9 (2026-11-16, C2 done flake): rerun local TestPerfHybridP95Budget10k 4x PASS (1x p50 59ms p95 66ms max 74ms + 3x suite ok, techo 250ms). Veredicto: flake — el fallo CI p95 261 vs 250 (+4% sobre techo 5x50ms) no reproduce en local y no hay cambio de código en el path de búsqueda; sin regresión. CI push 4ae6737 (run 43876004108) en curso al cerrar C2.
