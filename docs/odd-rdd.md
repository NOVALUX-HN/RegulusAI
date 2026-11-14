# ODD proporcional + RDD candidato congelado (NOV-53 / T34)

Sin reemplazar Sextant, Charts ni Gauntlet.

ODD:
- Autorizar → explorar → clasificar (incertidumbre/riesgo/resumibilidad) → registrar solo sustancial → implementar en unidades → verificar → cerrar.
- Pequeño y entendido: inline, sin Chart/documentos ni subagentes por defecto. Fuente única para sustancial: `odd/tasks/<feature>.md` (Specs Sx citan verbatim, Tasks Tx, Log Lx); workers reciben `Spec: path, Tn, Sx`, leen hasta `## Log`.
- Test-first si hay prueba determinista + resultado esperado; compara previo vs nuevo en comandos tocados; actualiza help/docs con el cambio.

RDD (opt-in, consentido, nunca auto-habilitado):
- Congela candidato: git base/head, worktree, archivos, exclusiones, hash. Revisión ligada a esa identidad; drift invalida, no se reusa.
- Riesgo determina profundidad; revisión no autoriza commit/push/release. Corrección acotada + reverificación contra candidato correcto.

Journeys: small (inline sin tracking) / large (feature doc + units) / high-risk (verifier independiente) y resume desde `git diff` + doc. Límites por tiempo/tokens/calidad, no por nº archivos. Sin regresión Sextant: charts + `regulus spec check` siguen decidiendo done.
