# Regulus-AI mejoras (Linear P-NOV-2)

## Specs
- S1: Flujo guiado Sextant AUTHORIZE → CHART → BUILD → PROVE → LEARN con evidencia por fase (NOV-38)
- S2: Benchmark reproducible calidad/precisión/costo como línea base (NOV-35)
- S3: Manifiesto canónico herramientas/comandos sin divergencias (NOV-25)
- S4: Contratos versionados Go–MCP–plugin (NOV-37)
- S5: Gate feedback explica qué falló, por qué importa y siguiente paso concreto; chart incompleto no pasa a BUILD; journey recorre todas las fases

## Tasks
- T1 NOV-38 Paso 1 flujo guiado Sextant | S1,S5 | route: inline | commit: uncommitted (task.go, gate_run.go, sextant_test.go, docs/sextant.md) — Linear Done 2026-11-14
- T2 NOV-35 Paso 2 benchmark | S2 | route: inline | commit: uncommitted (bench.go, cli/bench.go, bench_cost_test.go, baseline.json 14/14 proxy=472) — Linear Done 2026-11-14
- T3 NOV-25 Paso 3 manifiesto | S3 | route: TBD | commit: _
- T4 NOV-37 Paso 4 contratos | S4 | route: worker mwgoc3pa | commit: uncommitted (contract.go, version.go, manifest.go, contracts.md) — Linear Done 2026-11-14
- T5 NOV-27 Paso 5 métricas | S-calidad | route: worker mwgoplo6 | commit: uncommitted (crap.go, audit.go, gates.go, quality-exceptions.md) — Linear Done 2026-11-14

## Log
- L1: "que ondas compa, te comento que en linear en el proyecto de Regulus AI he dejado un plan de mejoras que quiero que lo revisemos para que entendamos que debemos hacer para mejorar regulus."
- L2: "procedamos pero al finalizar cada tarea quiero que en cada tarea finalizada de linear agregues que es lo que se hizo y el resultado que se obtuvo. es importante que actualicemos el estado de cada tarea en linear. procede paso a paso con las tareas"
- L3: T1 en curso — gap mínimo identificado por explorer: (1) mensajes task/gate sin tripleta qué/por qué/siguiente, (2) gate run no valida chart antes de BUILD, (3) sin journey AUTHORIZE→LEARN.
