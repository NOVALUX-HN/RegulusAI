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
- F3 (S3): tracking odd/tasks — route: inline — commit: bc6f098
- F4 (S4): reindex operativo — route: inline — commit: n/a (operacional, ver L4)
- F5 (S5): micro-deuda segura — route: inline — commit: n/a (evaluada, sin cambio: const pineada por diseño, Encode insignificante, Negotiate cosmético; ver L3)
- F6 (S6): verificación total + reporte — route: inline — commit: n/a (verificación, ver L5)

## Log

L5 (2026-10-31, F6 verificación): `go vet ./...` 0 + `go build ./cmd/regulus` 0 + `go test ./...` verde (28 pkgs) + `bun test --cwd .opencode` 56 pass + `opencode --version` v2.2.3 + `regulus doctor` V2 supported (unstamped dev build por diseño, `npm run build` estampa release). Todo verde.

L4 (2026-10-31, F4 reindex): `regulus memory reindex` 42 obs sha256 f97d179f + `memory doctor` fresh hybrid active + `status` memory fresh, evolution 0/10, 569 receipts. Stale de auditoría ya resuelto.

L3 (2026-10-31, F5 veredicto): los tres micros se evaluaron y se dejan como están: (a) mcpProtocolVersion en cli/mcp.go parece en desuso pero TestProtocolVersionPinned la lee del source a propósito — quitarla exigiría reescribir el test hacia el path servido (cross-package, scope creep sin ganancia funcional); (b) `_ = Encode` solo fallaría con conexión rota, sin nada útil que hacer; (c) Negotiate case-sensitive/duplicados es cosmético. Ninguno califica como "trivial y testeado" sin churn.

L1 (user original verbatim): "soluciona todos los problemas y mejoras en orden hasta finalizarlos todos, luego prueba todo nuevamente para verificar que las mejoras funcionaron"
