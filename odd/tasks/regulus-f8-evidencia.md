# Regulus F8 — Evidencia real y madurez (Linear P-NOV-2, F8 0%)
Cerrar 10 issues Backlog en develop, solo main+develop, comentando en Linear qué se hizo + resultado.

## Specs
- S1: "vuelve a crear la rama develop porque miro que se eliminó y usemola para ejecutar la nueva fase que se agregó en linear para mejorar regulus. revisa por favor"
- S2: "al finalizar cada tarea quiero que en cada tarea finalizada de linear agregues que es lo que se hizo y el resultado que se obtuvo"
- S3: F8 NOV-62 tokens reales por host (P0); NOV-68 evals con agente + holdout (P0)
- S4: F8 NOV-63 gate deriva documental (P1); NOV-64 trazabilidad issue-commit + revisión P0 (P1); NOV-69 conformidad hosts reales (P1); NOV-71 primer ciclo auto-mejora real (P1)
- S5: F8 NOV-65 fuzz/bench/strict env (P2); NOV-66 suite Windows completa (P2); NOV-67 modularización + límite 800 líneas (P2); NOV-70 releases verificables (P3)

## Tasks
- T42 NOV-62 Paso 42 tokens reales | S3 | route: inline | commit: 2ba77da Done
- T43 NOV-68 Paso 43 evals agente | S3 | route: inline | commit: 378e53f+ae7b155+21f58b5+c893845 Done
- T44 NOV-63 Paso 44 deriva docs | S4 | route: inline | commit: 74a10e4 Done
- T45 NOV-64 Paso 46 trazabilidad | S4 | route: inline | commit: 46817bf Done
- T46 NOV-69 Paso 45 conformidad real | S4 | route: inline | commit: 826235a Done (2 hosts reales pendientes de creditos)
- T47 NOV-71 Paso 50 ciclo real | S4 | route: inline | commit: 0804e72 Done (TIE+aprobacion humana: adopta recortados; rechazo real documentado)
- T48 NOV-65 Paso 47 fuzz/bench | S5 | route: inline | commit: b993bbf Done (2 fail-closed reales en allowlist; perf P95 queda fuera: entorno)
- T49 NOV-66 Paso 48 Windows full | S5 | route: inline | commit: f02e89b Done
- T50 NOV-67 Paso 49 modularización | S5 | route: inline | commit: 631a3bc+3aa6dad+ad21227+806ee4f+9378050 Done (dashboard embed byte-identico, memory/bench/gates split, domain.ts 9 modulos + plugin tests, gate size 16)
- T51 NOV-70 Paso 51 releases | S5 | route: inline | commit: b22264f+938d3c8 Done (tag push de prueba pendiente: decision de publicacion)

## Log
- L1: "vuelve a crear la rama develop porque miro que se eliminó y usemola para ejecutar la nueva fase que se agregó en linear para mejorar regulus. revisa por favor" — develop existe local+remoto en 9274004, se usa tal cual; F8 10 issues Backlog (NOV-62..NOV-71)
