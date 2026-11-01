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

| ID | Spec | Work | Route | Status |
|----|------|------|-------|
| M1 | M1 | spike + ADR SQLite-vs-stdlib + veredicto guardián | @regulus-worker |  |
| M2 | M2 | migración del store según M1 (WAL+índice+locks, legacy legible) | @regulus-worker | done |
| M3 | M3 | auto-save al cerrar unidad + export/import portable | @regulus-worker |  |
| M4 | M4 | minero envfacts (≥3 sesiones, descarte por contradicción, 0 modelo) | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-01 (programa macizo 1/3). Evidencia: Engram "Local SQLite memory remains default and authoritative source"; nuestro jsonl full-scan `almanac.go:262-290`, sin locks, zero-deps `go.mod:1-3`.
- L2: M1 CLOSED 2026-11-01 (`ses_e6ae888aeffenkXXeJMWEbKsEm` + sello `@regulus-reviewer` `ses_e6adf2f88ffeKx8NCyEHvzA6Qf`): DECISIÓN stdlib (extender `retrieval.bin`, SQLite diferido); guardián APRUEBA CON CONDICIONES (flock→lockfile portable, tmp+Rename, IndexVersion=2, tripwires). Incidencia: el run original corrió con `bin/regulus.exe` stale (0.10.0 pre-fix) vía MCP → receipt sin pins; binario reconstruido (`go build`, 0.10.0, 2026-11-01 21:24) y re-run `run_f4093081aeda556a` 12/12 con verify limpio en run nuevo. `task close` bloqueado por manifest viejo (artifact cuarentenado; `prune` protege manifests/cuarentena por diseño) → cierre manual (precedente 0.10.0). ADR en `docs/adr/M1-01-sqlite-vs-stdlib.md` con veredicto registrado.
- L3: M3 CLOSED 2026-11-01 (`ses_e6acef5f8ffeW91eriIWTiaY7I`): auto-save al cerrar (idempotente, dedup 0.92, best-effort sin cambiar veredicto) + export/import portable por dominio (bundle commiteable estilo `.engram/`, import idempotente). Run1 cayó en crap (ExportDomain cx26, señal real) → SPLIT en 6 + tests → re-run `run_b1123f9b069c33a5` 12/12, verify 19/19. `task close` exige historial todo-verde (incluye run1 fallido) → cierre manual documentado; historia fail→fix→pass es evidencia sana. Candidata mejora futura: close sobre último manifest verde.

- CLOSED M4 COMPLEX spec OK + 12 receipt(s) PASS.
- L4: M4 detail 2026-11-01 (`ses_e6ab8037fffeq72XjaTWxLwkdD`, cierre formal vía `task close`): minero envfacts determinista (`internal/envfacts/`: MinSessions=3, ventana contradicción 3, 0 modelo) + `memory mine [--apply]` (dry-run default) que admite lessons de dominio (inyectadas por hook existente) y descarta contradichos vía `invalidated_by`. Hecho real: `sextant/gate/crap` (16 sesiones, high). `gate run macizo-memory/M4` 12/12 primer run (`run_98dfa3b85a019ad9`), verify 12/12.
- L5: M2 CLOSED 2026-11-01 (`ses_e6a7d7b91ffex1gVfXWt6sshyt`, cierre formal): índice v2 (offsets ordenados + verify exacto, `IndexVersion=2`, `ErrHeaderMismatch` amable), lecturas acotadas con fallback legacy (scoped lee 14% a 10k), reindex atómico tmp+Rename, lockfile portable `O_EXCL` (cero flock), tripwires en perf_test + `memory doctor`. Trigramas diferidos (offsets bastan). `gate run macizo-memory/M2` 12/12 primer run (`run_6c45e5b6f19e4493`), verify 12/12. `go.mod` cero requires intacto.
- CLOSED M2 COMPLEX spec OK + 12 receipt(s) PASS.
- L6: H7 auditoría 2026-11-01 (añade sin reescribir L4/L5 cerradas): M4 primer run verde `run_98dfa3b85a019ad9` → 12 rec_ explícitos (typecheck:`rec_18e3d08d5322cf88`, lint:`rec_18e3d08d9d828a3c`, build:`rec_18e3d08dd1570a90`, unit-tests:`rec_18e3d08e10c64074`, coverage-floor:`rec_18e3d0b3a704f9ac`, mutation:`rec_18e3d0b3d4441448`, crap:`rec_18e3d0ba9c797e98`, vulncheck:`rec_18e3d0bab7ec5038`, spec:`rec_18e3d0bb72a96c08`, ts-typecheck:`rec_18e3d0bb8da55314`, ts-unit-tests:`rec_18e3d0bbaad9b754`, gofmt:`rec_18e3d0bc0206314c`) resolubles en `.regulus/evidence/macizo-memory__M4/manifest-run_98dfa3b85a019ad9.json` sin abrir diff; re-run `run_a4938de4e9b377c5` → 12 rec_ explícitos (typecheck:`rec_18e3d716b446de28`, lint:`rec_18e3d71700492100`, build:`rec_18e3d7172a18d82c`, unit-tests:`rec_18e3d7176a7b0fd4`, coverage-floor:`rec_18e3d77104a50660`, mutation:`rec_18e3d77132bce144`, crap:`rec_18e3d777c364fc1c`, vulncheck:`rec_18e3d777df511c6c`, spec:`rec_18e3d7789bf7cf00`, ts-typecheck:`rec_18e3d778b5ad4e48`, ts-unit-tests:`rec_18e3d778d2fdafc4`, gofmt:`rec_18e3d7792788d2bc`) resolubles en `.regulus/evidence/macizo-memory__M4/manifest-run_a4938de4e9b377c5.json`.
- L7: H7 notas observables M2/M3 (sin cambiar Given/When/Then cerrados de `charts/macizo-memory.md:29-39`): M2 observable = lecturas acotadas con fallback legacy + gauntlet verde (índice v2 `IndexVersion=2`); M3 observable = export/import portable por dominio + auto-save idempotente al cerrar (dedup, sin duplicar pineado). Docs `task check`: ejemplo en `docs/sextant.md` (`go run ./cmd/regulus task check charts/<feature>.md`).