# macizo-memory — SQLite + hábito Engram + envfacts

User authorization: "crea un plan para ejecutar cada uno de ellos e integrarlos a Regulus para hacer un producto macizo; considera SQLite local para Almanac/Engram en vez de archivos" 2026-11-01. Programa macizo 1/3 (memoria). Read down to `## Log` before working.

Class: COMPLEX

Domain: almanac

## Contract

- Objective: memoria interna funcional sobre SQLite (o índice stdlib si el ADR lo decide), hábito de guardado automático estilo Engram y hechos envfacts minados sin modelo.
- Scope: `internal/almanac/*`, `internal/library/*`, `internal/cli/memory.go`, `.opencode/plugins/regulus.ts` (hooks), `docs/memory*.md`, `go.mod`+`TestZeroDepsGoMod` (solo se tocan si el ADR lo aprueba con guardián). Sin cambiar formato de receipts; compat de lectura legacy obligatoria.
- Constraints: el append-only como WAL se preserva; sin cgo jamás (rompe 6 targets); si entra dependencia pure-Go, `docs/audit/dependency-map.md` + CI + test se actualizan en el mismo cambio.
- Acceptance: M1, M2, M3, M4
- Artifacts: receipts `macizo-memory/M*` (12/12 por tarea).
- Dependencies: M2 tras M1 (decisión ADR); M3–M4 independientes.
- Risk: complex
- Verification: `spec check` + `gate run macizo-memory/M*` 12/12 + guardián en M1.
- Completion: M1–M4 CLOSED vía `task close`.

## Acceptance

### Scenario: M1

- Given el store jsonl con full-scan por query (`internal/almanac/almanac.go:262-290`), sin locks (`budget.go:100` convención single-writer) y el invariante zero-deps (`go.mod:1-3`, `TestZeroDepsGoMod`)
- When se corre un spike (medir tamaños reales, prototipar modernc-sqlite vs índice stdlib en /tmp fuera del árbol) y se escribe el ADR
- Then la decisión SQLite-vs-stdlib queda documentada con trade-offs, coste de migración y veredicto de guardián

### Scenario: M2

- Given la decisión M1
- When se migra el store (WAL append compatible + índice ordenado/FTS + locks donde haya escritores)
- Then lecturas acotadas (sin full-scan), legacy legible, conducta idéntica y gauntlet verde

### Scenario: M3

- Given el guardado manual (`memory note` → `TypeSessionNote`, `memory.go:263-265`) y que Engram guarda sin ceremonia con `sync` portable
- When cada cierre de unidad auto-guarda su observación y existe export/import portable del dominio
- Then el hábito es automático, el sync entre máquinas funciona y nada duplica lo ya pineado

### Scenario: M4

- Given receipts de gates que fallan + traces con fingerprints por sesión, sin minería hoy
- When se minan fallos repetidos (mismo fallo en ≥3 sesiones) como hechos de dominio con descarte por contradicción
- Then los hechos se inyectan por dominio al inicio de sesión con cero llamadas al modelo, reversibles

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| M1 | M1 | spike + ADR SQLite-vs-stdlib + veredicto guardián | @regulus-worker |
| M2 | M2 | migración del store según M1 (WAL+índice+locks, legacy legible) | @regulus-worker |
| M3 | M3 | auto-save al cerrar unidad + export/import portable | @regulus-worker |
| M4 | M4 | minero envfacts (≥3 sesiones, descarte por contradicción, 0 modelo) | @regulus-worker |

## Log

- L1: Chart opened 2026-11-01 (programa macizo 1/3). Evidencia: Engram "Local SQLite memory remains default and authoritative source"; nuestro jsonl full-scan `almanac.go:262-290`, sin locks, zero-deps `go.mod:1-3`.
- L2: M1 CLOSED 2026-11-01 (`ses_e6ae888aeffenkXXeJMWEbKsEm` + sello `@regulus-reviewer` `ses_e6adf2f88ffeKx8NCyEHvzA6Qf`): DECISIÓN stdlib (extender `retrieval.bin`, SQLite diferido); guardián APRUEBA CON CONDICIONES (flock→lockfile portable, tmp+Rename, IndexVersion=2, tripwires). Incidencia: el run original corrió con `bin/regulus.exe` stale (0.10.0 pre-fix) vía MCP → receipt sin pins; binario reconstruido (`go build`, 0.10.0, 2026-11-01 21:24) y re-run `run_f4093081aeda556a` 12/12 con verify limpio en run nuevo. `task close` bloqueado por manifest viejo (artifact cuarentenado; `prune` protege manifests/cuarentena por diseño) → cierre manual (precedente 0.10.0). ADR en `docs/adr/M1-01-sqlite-vs-stdlib.md` con veredicto registrado.
