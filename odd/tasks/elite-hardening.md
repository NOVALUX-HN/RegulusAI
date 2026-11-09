# elite-hardening — llevar el harness al máximo nivel con fixes puntuales y medidos

Header: fixes puntuales y eficientes sobre velocidad, precisión, decisiones y tokens, con tabla comparativa probada. Branch: feat/elite-hardening.
Read until `## Log` before edits.

## Specs

- S1: "soluciona todos los problemas de forma puntual, pero eficiente."
- S2: "arreglamos y muestrame al final y una tabla comparativa ya probada y documentada de los cambios que se hicieron y las mejoras que esto nos trajo."
- S3: "recordar que este harness quiero que sea para programar al mas alto nivel"

## Tasks

- [x] T1 (S1,S3) — velocidad: quick útil + mutación contenida — route: inline — commit: 63cec17
- [x] T2 (S1,S3) — precisión CRAP: partir loadLiveCache + cubrir envfacts — route: inline — commit: 46919c0
- [x] T3 (S1,S3) — precisión evidencia: aislamiento degradado explícito + spec floor — route: inline — commit: cdcca88
- [x] T4 (S1,S3) — decisiones: scoring cuantitativo + 2 bench tasks de decisión — route: delegated — commit: 04b5fb0
- [x] T5 (S1,S3) — tokens/i18n: loopguard en inglés + warning de fallback de dominio + budget doc — route: inline — commit: 1723ebb
- [x] T6 (S2) — tabla comparativa probada y documentada — route: inline — commit: 045d1f7
- [x] T7 (S2,S3) — verificación final elite: journeys + full suite — route: inline — commit: verification-only (no code)

## Log

- L1: "soluciona todos los problemas de forma puntual, pero eficiente. arreglamos y muestrame al final y una tabla comparativa ya probada y documentada de los cambios que se hicieron y las mejoras que esto nos trajo. recordar que este harness quiero que sea para programar al mas alto nivel"
- L2: T1 done 63cec17 — RED malformed JSON (doble coma), GREEN bench B9 + dry-run quick=10/full=12
- L3: T2 done 46919c0 — split loadLiveCache en 4 helpers, CRAP 29.91→25.91 entonces, helpers 100%, pkg 85.7%
- L4: T3 done cdcca88 — aislamiento degradado explícito, 5 tests docker PASS
- L5: T4 done 04b5fb0 (delegated) — Score 0-100 + B12/B13, bench 13/13 49ms, decisions+bench GREEN
- L6: T5 done 1723ebb — loop-guard EN + domain fallback warning, bun 105+148 PASS
- L7: T6 done 045d1f7 — docs/elite-hardening-results.md + pines cli 11→13
- L8: T7 verification-only — decisions/bench/envfacts GREEN, bun 148 PASS, spec 202 scenarios OK, crap max 27.45 <30, quick=10/full=12
