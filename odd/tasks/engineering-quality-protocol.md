# engineering-quality-protocol — F20 · Engineering Quality Protocol & Verification Effectiveness

Linear: NOV-132 (padre), NOV-150..NOV-153 (hijas). Baseline: `develop`, nunca `main`.

Leída la fase padre y **cada hija completa**. Estado de arranque: rama `develop`,
árbol limpio, sin empezar.

## Specs

S1 — Aplicar un REQP transversal **sin crear un nuevo reviewer ni un stack de
tooling nuevo**. Texto verbatim del padre: "sin crear un nuevo reviewer/tooling
stack". Reutilizar reviewer, audit, gates, coverage, mutation, fuzzing, CI y
métricas de calidad existentes (NOV-27, NOV-28, NOV-41, NOV-65, NOV-67, NOV-84,
NOV-83, F11 y el Gauntlet actual).

S2 — REQP operativo y adaptado a riesgo y lenguaje, con excepciones justificadas:
subtract, premise, root cause, TDD cuando sea viable, unidades pequeñas,
behavior testing, boundaries, idempotency, evidence, regresión, rollback.

S3 — Tooling decidido por evidencia: compatibilidad, licencia, mantenimiento,
seguridad y rendimiento. Preferir lo nativo existente y **no integrar si el coste
no se justifica**. Cada decisión queda como decision record.

S4 — Hotspots de calidad y risk scoring; **nunca métricas vanity ni cobertura
uniforme ciega**.

S5 — Mutation testing selectivo en paquetes críticos, con sobrevivientes
clasificados y triaje (gap de test vs coste).

S6 — Duplicación estructural, complejidad y dead code: **report-only** al inicio,
remediación proporcional.

S7 — Gates y reviewer verifican diff, tests, invariantes, evidencia, regresiones y
claims; no se vuelven arbitrarios.

S8 — Fixtures de efectividad de reviewer y gates, incluyendo fallos adversariales.

S9 — Texto verbatim de las exclusiones: "No exigir mutation/coverage universales;
no refactor masivo; no instalar UB Mode como dependencia runtime; no aceptar
metrics vanity."

S10 — DoD verbatim: "REQP documentado/integrado; tooling decision records; métricas
con provenance; mutation outcomes en critical packages; auditoría effectiveness;
security/false positive checks; baseline comparison; Gauntlet, regression y
independent review."

S11 — Si no hay brecha demostrada, no se inventa trabajo: se reporta y se propone
cierre como audit-only.

## Tasks

| ID | Linear | Specs | Qué pide |
|----|--------|-------|----------|
| T1 | NOV-150 | S1 S2 S11 | Auditar Sextant/BUILD/PROVE/LEARN, reviewer, audit, gates y DoD; integrar REQP |
| T2 | NOV-151 | S3 S4 S6 | Evaluar métricas CRAP/duplicación/dead code y su tooling |
| T3 | NOV-152 | S5 S10 | Efectividad de mutation testing en rutas críticas |
| T4 | NOV-153 | S7 S8 | Efectividad de reviewer, gates y verificación |

### Estado de T1 (NOV-150)

Auditoría entregada en `docs/reqp-integration-audit.md` (`e22fc53`). Conclusión:
mitad de REQP ya bloquea (gates requeridos, barrera de chart, contrato de cierre,
receipts, idempotencia) y la otra mitad es prosa.

Integración elegida, mínima: `internal/protocol` registra las once prácticas con
su nivel real — `enforced`, `advisory` o `judgement` — y **ancla cada cita a un
marcador que debe seguir existiendo en el archivo**. `regulus protocol` lo imprime
y se niega a imprimir una tabla inválida. `docs/engineering-protocol.md` es la
página única que el protocolo no tenía.

Lo que deliberadamente **no** se hizo: siete gates nuevos para prácticas sin
oráculo, tocar el contrato de receipts del reviewer, o exigir que los charts
declaren sus prácticas. Los tres habrían roto invariantes para parecer completos.

### Estado de T2 (NOV-151)

Inventario: `crap`, `coverage-floor`, `size` y `lint` **ya son gates requeridos**
(`regulus.json`), así que complejidad, CRAP y cobertura no tienen brecha y no se
tocan. Lo que no existía era medición de **duplicación** ni de **dead code**.

Decisión de tooling en `docs/quality-tooling-decisions.md`, con el dato que la
condiciona: `go.mod` **no tiene ni un `require`**. Agregar cualquier analizador
sería la primera dependencia externa del proyecto — una decisión de entrega, no de
calidad.

Adoptado: `internal/duplication`, nativo, report-only, `regulus duplication`.
Dead code: evaluado y **diferido con el follow-up nombrado** (`deadcode` detrás de
un gate no requerido cuando el proyecto acepte su primera dependencia de análisis);
las versiones nativas baratas son máquinas de falsos positivos por satisfacción de
interfaces, reflexión y build tags. Rechazados con motivo y licencia: `dupl`,
`x/tools`, `jscpd`, `staticcheck`, y `golangci-lint` por **GPL-3.0**.

Hallazgo real del detector sobre el repo: `generateServeToken` comparte estructura
con `MintID`, `mintExpID`, `mintGovID`, `mintValID` y `mintLabSession` — la familia
completa de acuñadores de tokens. Eso es lo que el report-only sirve: hacer visible
el patrón para que un humano decida, sin convertirlo en un umbral arbitrario.

T1 abre la fase porque condiciona a los otros tres: si REQP no está integrado en el
flujo, medir tooling (T2), mutation (T3) o reviewer (T4) no tiene dónde engancharse.

## Advertencia de alcance

La fase pide "evolucionar" siete superficies existentes. El riesgo real es
convertirla en un refactor masivo, que las exclusiones prohíben. Cada tarea debe
demostrar su brecha con evidencia antes de tocar código, y cerrar audit-only si no
la hay.

## Patrón de trabajo (el que funcionó en F16–F19)

1. Leer la issue padre y **cada** hija completa; el requisito real vive en
   `## Objetivo` (las cuatro comparten un bloque genérico de Acceptance criteria).
2. Verificar rama, HEAD, árbol y `origin`.
3. Auditoría read-only primero.
4. Implementación mínima con tests que intentan romper la garantía.
5. `bash scripts/verify-wsl.sh <paquetes>` + `go vet`.
6. Commit con la brecha y la evidencia en el mensaje.
7. `bash scripts/regulus-wsl.sh gate run --quick --yes <fase>/<tarea>` (5–9 min,
   en background).
8. Revisión independiente con `gentle-ai-verify`; en F16–F19 encontró defectos
   reales **todas las veces**, incluido un regreso propio.
9. Cerrar en Linear con evidencia y límites declarados.

## Trampas ya pagadas

* `docdrift`: un documento que diga "N tools" afirma el total de la superficie.
* Pin del dispatch: agregar un comando exige subir `TestRunDispatchTableCompleteness`
  (hoy 41) con la razón al lado.
* `TestNoRawProcessLaunch`: todo spawn por `internal/pxexec`.
* Sin código sin consumidor en producción.
* Nunca `--amend` sobre un commit que ya está en `origin`.
* Backticks en `git commit -m` son sustitución: usar `git commit -F -` con `<<'MSG'`.
* Tokens son proxy: etiquetar `proxy`/`estimated`/`unknown`, nunca `measured` sin
  evidencia.

## Log

L1: "me ire a dormir, así que necesito que no solo finalices el f19 completo, sino
que al finalizar traigas las siguientes fases y analices cada issue como lo hemos
hecho con su informacion completa de cada uno y las hagas hasta finalizar todas las
fases que creamos con sus test funcionando a cabalidad."

L2: F19 se cerró completa (4 tareas, Gauntlet de fase 6/6). F20 quedó **analizada
pero no implementada**: el presupuesto de contexto de la sesión se agotó justo
después. Este documento es el análisis que pediste, listo para ejecutar.

L3: NOV-151 ya no dice "Crapper/Dryer": el título es ahora "Evaluate CRAP,
duplication and dead-code metrics/tooling". La corrección que pedí en el comentario
de mejoras quedó aplicada, así que esa parte del hallazgo está cerrada.

L4: T1 implementado. La auditoría había mostrado que la brecha real no era "falta
un protocolo" sino "el protocolo está repartido y la mitad de sus prácticas no
tiene diente". La integración ataca eso en el único punto honesto: la tabla de
prácticas con su nivel de enforcement real, y un test que rompe si una cita deja de
existir. Verificado: `go build ./...` 0, `go vet` 0, `gofmt` limpio.

L5: T2 implementado tras inventario. Cuatro señales ya estaban gateadas y no se
tocaron (subtract before you add). El módulo no tiene dependencias: eso decidió
nativo para duplicación y diferido para dead code, en vez de agregar el primer
`require` para una métrica.

L6: T3 medido con gremlins sobre cuatro paquetes. La tabla resultó ser el hallazgo:
la eficacia sigue a los gates. `receipts` 100% y `gates` 98.64% ya tenían gate; los
dos que no lo tenían — justo los que sostienen las afirmaciones de integridad —
estaban en 78.4% (`evidence`) y 77.83% (`recovery`).

Cerrados con test y re-medidos: 4 mutantes de borde de charset en `validateGateID` y
**7 en `isWindowsAbsolute`**, una guarda de escape de rutas sin un solo test directo.
`gates` 98.64% → 99.73%, `evidence` 78.40% → 84.00%.

Equivalente con prueba, no con intuición: `quick_lean.go:35`, porque `validate`
rechaza layers duplicados y ése es el único input donde `<` y `<=` difieren. No se
toca el código.

Abiertos y clasificados: 47 en `recovery`, agrupados en `progress.go:184` y
`outcome.go:149/152` — umbrales de presupuesto de F17, trabajo propio.

Gates: cuatro, todos `required:false` y sólo `full`, con umbrales medidos como piso
anti-regresión (receipts/gates 70→95, evidence 82, recovery 76), no como meta.
Coste medido total: ~9m40s. Las capas 24 y 25 porque 9 y 10 ya eran `crap` y
`vulncheck`, y el validador rechaza duplicados.
