# pro-harness — Regulus hacia harness de calidad profesional

Lo mejor de gentle-ai (RDD con autoridad), harness2 (medición de mejora), ponytail (ladder completo + audit) y TokenEconomizer (poda por uso, proposer periódico, control de costo), en fases que cada una deja el repo en verde. Orden: medir → barato → transformador.

## Specs

- S1: "haz un plan para implementar lo mejor de cada uno de esos universos para hacer que regulus sea un harness de calidad profesional"
- S2: "procede y crea test o modifica los existentes para probar que cada cambio este completo y funcional"
- S3: Fases aprobadas en orden 0 → 1 → 2 → 3 → 4 → 5 (medir, ladder+audit, poda por traces, proposer periódico + contradicción, reporte de evolución, RDD con autoridad). Cada fase shipeable sola; frenar en cualquier cierre deja un harness mejor.
- S4: Cada cambio con test nuevo o modificado que pruebe completitud y funcionalidad; tsc + suite verde al cierre de cada fase; sin commits salvo work-units en feature branch; no reescribir el gauntlet; no juez-modelo sin evidencia.
- S5: Engram mirror no disponible (provider caído: engram.exe bloqueado por WDAC) — este documento es la única fuente de continuidad; al reanudar, leerlo entero antes de tocar código.

## Tasks

- T0 (S1,S2,S4): Fase 0 — bench compara con/sin lección + baseline en charts/ — ruta: inline — commit: d206fb3
- T1 (S1,S2,S4): Fase 1 texto — ladder 6 peldaños + reviewer 4 campos en agents — ruta: inline — commit: 707d745
- T2 (S1,S2,S4): Fase 1 audit — comando rankeado repo-wide sobre CRAP/traces — ruta: inline — commit: 15cf4fc
- T3 (S1,S2,S4): Fase 2 — poda de toolset por uso medido en traces — ruta: inline — commit: pendiente
- T4 (S1,S2,S4): Fase 3 — hook proposer cada k sesiones + supersede por contradicción — ruta: inline — commit: pendiente
- T5 (S1,S2,S4): Fase 4 — evolution report con delta por lección — ruta: inline — commit: pendiente
- T6 (S1,S4): Fase 5 diseño — chart RDD (lineage, lenses, budget, burn, consent) — ruta: inline — commit: pendiente
- T7 (S1,S2,S4): Fase 5 core Go — transacción de review — ruta: inline — commit: pendiente
- T8 (S1,S2,S4): Fase 5 plugin + migración guardians→lenses — ruta: inline — commit: pendiente

## Log

- L1 (origen, verbatim): "quiero que analices todo regulus y me digas si el harness contempla lo mejor de lo siguiente: https://github.com/Gentleman-Programming/gentle-ai https://github.com/google-research/harness2 https://github.com/DietrichGebert/ponytail https://github.com/microsoft/TokenEconomizer analiza bien a detalle si tenemos lo mejor de todo eso" → análisis entregado con 5 gaps rankeados; "haz un plan para implementar lo mejor de cada uno de esos universos para hacer que regulus sea un harness de calidad profesional" → plan de 6 fases aprobado con "procede y crea test o modifica los existentes para probar que cada cambio este completo y funcional".
- L2: Rama feat/pro-harness creada sobre develop con árbol sucio heredado (6 archivos de EUNKNOWN/stale-core + charts/plugin-resilience.md sin trackear, de sesiones previas, verificados y en uso por opencode — NO tocar ni commitear; los commits de esta feature solo stagean archivos propios).
- L3 (T0 evidencia): B11 lesson-context-cost golden — fixture 2 notas + 1 lección con evidencia arma 550 chars = 138 proxy tokens (chars/4, host unknown), hermético (ContextScoped solo lee temp store, timestamps fijos). bench/baseline.json regenerado a macizo-bench-v3 11/11; `bench compare` → ADOPT. Go verde salvo internal/portability (preexistente ambiental: bashPosixPath asume WSL /mnt/c, acá Git Bash /c/ — fuera de scope T0, no tocar). Bun 138/138, tsc limpio.
- L4 (slices): T0 d206fb3 (139+/22-) y T1 707d745 (59+/8-) — cada unidad bajo el corte de ~400. El aviso de "2054 líneas en 3 commits since origin/main" es drift heredado develop-vs-main + árbol sucio previo, no de esta feature.
- L5 (T2 evidencia): `regulus audit --profile --top --max` — entry points (cmd/*) + tiers must-fix (>max compuerta)/should-fix (cov 0 + cx≥5)/nice + veredicto "fix X first", exit 0 informativo. Dogfood real: 0 must-fix | 0 should-fix | 8 nice, top loadLiveCache CRAP 29.91 (bajo el max 30, consistente con la compuerta). Pin de dispatch actualizado deliberadamente. Go verde salvo portability preexistente; bun 140/140.
