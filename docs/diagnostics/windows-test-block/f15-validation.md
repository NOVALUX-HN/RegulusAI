# F15.6 — Validación integral en Windows afectado

Fecha: 2026-11-17. Host: Win11 Pro 26300, go1.27.3, usuario no elevado,
integridad Medium, commit de validación más abajo. Protecciones intactas
(ningún cambio al sistema).

## 1. Alcance ejecutado

- `testflow run --mode full`: 50 paquetes descubiertos, 50 ejecutados
  (+2 checks build/vet). Dos corridas completas desde procesos nuevos
  (N=19 pre-fix descubrimiento, N=50 post-fix): consistentes.
- Evidencia: `evidence/testflow-full-2026-11-17.json` (veredictos por
  paquete) + logs de corrida (no commiteados, efímeros).

## 2. Resultado: 37 PASS / 12 BLOCKED / 1 FAIL (no verde)

- PASS 37, incluidos mbench (post-fix SafeRefusal), pxexec-vía-harness,
  testflow-unit (vía focused), mission, cmdx, govern, bench-agenttasks.
- BLOCKED 12: context, fsx, impact, library, planning, recovery, verify,
  bench, cli?, evolve, gates, mission, observability, pxexec, receipts,
  reflect, runtime, spec, testflow (binarios frescos denegados por SAC
  VerifiedAndReputable; 40 eventos 3077 en la ventana).
- FAIL 1 real y propio: `TestRunDispatchTableCompleteness` (36 fijos vs
  38 reales: faltaban `govern` de F13.6 y `testflow` de F15.4) — corregido
  en el acto. FAIL 1 ambiental: checks de bench que ejecutan fixtures
  (Infra por SAC, 0 fallos funcionales en checks ejecutados).
- Corrección propietaria: `TestSuitePassesExploration` acepta SafeRefusal;
  refactor `RunCase` en 7 fases (<100 líneas, gate de 200 cumplido);
  descubrimiento `go list` por pipe (multi-archivo ya no se pierde).

## 3. Verificaciones pedidas por el spec

- Smoke→focused→full: sí (smoke pxexec, focused mbench, full 50).
- Repetición desde proceso nuevo: sí (2 full + focused repetidos).
- build/vet por separado: PASS, no confundidos con tests.
- Descubiertos/ejecutados/pasados/fallidos/bloqueados/omitidos: contados
  (0 omitidos sin justificar; skips: ninguno).
- Árbol de procesos, exit codes, timeouts, sin huérfanos: runner los
  captura por paquete (ver JSON).
- Eventos de seguridad: 3077 correlacionados por ventana (40/30min);
  sin cambios a políticas ni exclusiones.
- Local vs CI: CI limpia no valida este host (dicho en el gate y aquí);
  la suite del host afectado queda BLOCKED donde SAC deniega.

## 4. Conclusión (honesta, según aceptación)

La infraestructura valida (descubrimiento completo, clasificación,
veredicto estricto, JSON reproducible) y el código migrado funciona
(37 PASS). La suite **no se declara resuelta**: 12 paquetes BLOCKED por
política externa + bench-fixtures Infra. Per spec: el resultado es
BLOCKED documentado, no PASS. Cierre pendiente de host limpio o
autorización administrativa (fuera de este entorno).
