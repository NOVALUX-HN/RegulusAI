# NOV-26 — No abstraer runtime sin segundo consumidor (T19)

Decisión: diferir abstracción. No hay caso de uso real.

Evidencia:
- `internal/runtime/doc.go:1-14` core stdlib-only, runtimes fuera vía `Adapter`.
- `internal/runtime/adapter.go:58-75` `Adapter{Name,Capabilities,Open}` mínimo testeable.
- `internal/runtime/registry.go:3-10` registro vacío por diseño; OpenCode vive en `.opencode/plugins/regulus.ts`.
- `internal/runtime/boundary_test.go:9-` pin `go list -m all==regulus-ai` + stdlib-only.
- Cero imports prod a `regulus-ai/internal/runtime` en cli/gates/evidence; contrato en `internal/contract/contract.go:7`.
- `docs/adr/T7-01-adapter-contract.md:15` solo `opencode`, resto unclaimed.

Si aparece segundo runtime: implementar adapter, registrar, probar equivalencia con journeys NOV-40. Hoy: documentado, sin capas nuevas.
