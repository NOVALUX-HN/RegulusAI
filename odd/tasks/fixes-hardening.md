# Fixes Hardening (post-auditoría)

Header: Cerrar los gaps del final-audit en orden: cap global de
experimentos, log de decisiones de guardians, tracking de odd/,
reindex operativo, micro-deuda segura y verificación total.

## Specs

S1: "Experimentos fuera del cap global: las corridas de evolution deben
consumir el ledger diario como el resto (nuevo Kind/Outcome, enforcement
en RunExperiments, tests de rechazo por cap)."
S2: "Guardian disagreement sin store: log de decisiones de guardians
(.regulus/guardians/decisions.jsonl) que la métrica agrega; sin log, 0
con coverage note (honestidad actual preservada)."
S3: "odd/ gitignored: versionar odd/tasks/ para trazabilidad (incluye
legacy presente en disco, inofensivo e histórico)."
S4: "memory reindex stale: correr reindex operativo y verificar status."
S5: "Micro-deuda solo si trivial y testeada: const mcpProtocolVersion en
desuso, Encode descartado en httpapi, Negotiate estricto (este último
cosmético: solo si no cambia comportamiento)."
S6: "Verificación total: go vet + build + go test ./... + bun + un probe
opencode v2, todo verde antes de reportar."

## Tasks

- F1 (S1): cap global de experimentos — route: delegated (gentle-ai-worker) — commit: 8d2d83e
- F2 (S2): guardian decision log + métrica — route: delegated (gentle-ai-worker) — commit: 23c0602
- F3 (S3): tracking odd/tasks — route: inline — commit: —
- F4 (S4): reindex operativo — route: inline — commit: —
- F5 (S5): micro-deuda segura — route: inline — commit: —
- F6 (S6): verificación total + reporte — route: inline — commit: —

## Log

L1 (user original verbatim): "soluciona todos los problemas y mejoras en orden hasta finalizarlos todos, luego prueba todo nuevamente para verificar que las mejoras funcionaron"
