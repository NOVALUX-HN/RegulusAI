# plugin-resilience — el plugin sobrevive a WDAC + stale core sin adoptar binario malo

User authorization: "quiero que aprendas de este error que acabamos de tener para evitar que vuelva a suceder y para asegurar que los proximos cambios no vuelvan a afectar tu funcionamiento" 2026-11-04. Error: `Server plugin error ... resolveRegulus ... EUNKNOWN uv_spawn bin/regulus.exe + bin/regulus, ENOENT binaries/regulus-win32-amd64.exe, stale core 0.8.1 vs 0.12.0`. Fix vía gentle-shell fue ambiental (fuera del repo). Read down to `## Log` before working.

Class: STANDARD

Domain: sextant

## Contract

- Objective: que un bloqueo WDAC (EUNKNOWN -4094) + fallback a `regulus` viejo en PATH nunca tumbe la sesión sin diagnóstico accionable, y que el alias `win32` → `windows` en `binaries/` no se vuelva a perder entre launcher y plugin.
- Scope: `.opencode/lib/domain.ts:356-370` (`candidates`), `bin/regulus.js:60-99` (orden + alias `win32` → `windows`), `.opencode/lib/domain.test.ts:522-547` (tests candidates), `docs/troubleshooting.md:96-151` (EUNKNOWN + stale core) y `docs/troubleshooting.md:193-195` (orden de búsqueda). Sin tocar `opencode.json`, `regulus.json#allowlist`, gates, `yolo.go`, ni habilitar `webfetch`.
- Constraints: no cambiar denies destructivos ni guardians read-only; primer match gana (S2) y fail-closed ante skew (S3) se preservan; `ENOENT` con token `uv_spawn` nunca se diagnostica como bloqueo; sin cgo jamás; explicación de bloqueo solo ante firma exacta (`EUNKNOWN/UNKNOWN/-4094`).
- Acceptance: R1, R2
- Artifacts: receipts `plugin-resilience/R*` (12/12 por tarea).
- Dependencies: R1 antes que R2 (R2 cita el orden final de R1).
- Risk: standard
- Verification: `spec check charts/plugin-resilience.md` + `gate run plugin-resilience/R*` 12/12 + guardian `@regulus-spec-keeper` en Chart y `@regulus-verifier` en R1.
- Completion: R1–R2 CLOSED vía `task close`.

## Acceptance

### Scenario: R1

- Given `.opencode/lib/domain.ts:356-370` construye `binaries/regulus-${platform}-${arch}` sin alias mientras `bin/regulus.js:70-72` sí prueba `regulus-windows-${arch}.exe` en `win32`, y `binaries/` solo trae `regulus-windows-amd64.exe` + `regulus-windows-arm64.exe` (sin `regulus-win32-*`), por eso el setup reportó `binaries/regulus-win32-amd64.exe: ENOENT`
- When `candidates()` conoce ambos nombres en `win32` en el mismo slot y orden que el launcher (primario `win32` + alias `windows`, sin alterar el orden 1-6 ni el primer-match-gana)
- Then en Windows el slot `binaries/` resuelve al `windows-*.exe` existente, `bun test ./.opencode/lib/` pinnea primario + alias + orden + caída a PATH, y `gate_run` no adopta un core stale

### Scenario: R2

- Given el setup falló con `EUNKNOWN uv_spawn` en `bin/regulus.exe` + `bin/regulus` (bloqueo WDAC, no PATH), `ENOENT` en el slot `binaries/` por R1, y `stale core: binary says 0.8.1 but package.json says 0.12.0` desde PATH (`docs/troubleshooting.md:131-151`), y `docs/troubleshooting.md:193-195` describe el orden sin nombrar el alias `win32` → `windows`
- When la guía nombra el alias en el orden de búsqueda y reitera las salidas reales (excepción por hash/firmante con eventos 3033/3077, correr en WSL con `binaries/regulus-linux-*`, máquina no gestionada; `Unblock-File`/recompilar/`go run` con `go.exe` bloqueado no sacan de WDAC)
- Then un operador con el mismo error llega a la salida sin probar fantasmas PATH/cwd, y `spec check` + `gate_run` siguen verdes

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|
| R1 | R1 | `candidates()` alias `win32` → `windows` en el mismo slot + tests bun (primario, alias, orden, PATH-last) | @regulus-worker | done |
| R2 | R2 | docs troubleshooting: alias en orden + salidas WDAC/stale sin fantasmas + verificación | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-04 desde error pegado por usuario (EUNKNOWN `bin/regulus.exe` + `bin/regulus`, ENOENT `binaries/regulus-win32-amd64.exe`, stale `0.8.1 vs 0.12.0` en `pickRegulus` `.opencode/plugins/regulus.ts:186-197`) + scout `ses_e5b7ef177ffeLsDS1L0r9Kckja` (launcher alias `bin/regulus.js:70-72`, `domain.ts:356-370` sin alias, `binaries/` solo `windows-*`, docs `troubleshooting.md:96-151`). Sin `question` adicional: el síntoma + fix ambiental ya están dados.

- CLOSED R1 STANDARD spec OK + 12 receipt(s) PASS (run_61a20d5e2d2bfea9).
- CLOSED R2 STANDARD spec OK + 12 receipt(s) PASS (run_d2ef6efd79699648).