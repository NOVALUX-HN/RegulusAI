# Suite completa en Windows (NOV-66 / T49)

`scripts/test-windows.ps1` corre la suite completa paquete por
paquete: compila cada test a `.regulus-test/*.test.exe` dentro del
workspace (nunca `%TEMP%`) y lo ejecuta. Si App Control bloquea un
binario igual (eventos 3033/3077/3089/3118), el paquete se marca
BLOCKED con el diagnóstico accionable (no un "permission denied"
mudo) y el exit solo falla por rojos reales. `GOTMPDIR` apunta al
workspace para no ensuciar Temp.

CI (`windows.yml`, job `windows-native`): build multiplataforma, vet,
suite completa vía el script, hashes SHA-256 publicados como
artefacto, y aserción de que el EXE sigue `NotSigned` mientras la
firma es opcional (NOV-44). El job `windows-protected-audit` vuelca
CodeIntegrity a artefacto (90 días): auditoría, nunca enforcement.

## Exclusiones clasificadas (con justificación, sin silencios)

| Caso | Comportamiento | Justificación |
|------|----------------|---------------|
| Bloqueo App Control de `*.test.exe` | BLOCKED + diagnóstico, no FAIL | Política empresarial del host, no del código; ver `docs/windows-diagnosis.md`. Laboratorio o WSL para el verde. |
| `TestUntarStripsExecBits` (POSIX chmod) | skip sin POSIX | Windows usa ACLs, no bits Unix. Por diseño. |
| Tests de symlinks (`scope_test.go`) | skip si el OS lo prohíbe | Requieren privilegio de desarrollador; skip con motivo, no silencio. |
| Tests `bash` | requieren Git Bash | Presente en runners `windows-2022`; en dev instalar Git for Windows. Con `REGULUS_STRICT_ENV=1` la ausencia falla en vez de omitir. |
| Tests `git` | requieren git | Presente en runners y dev. Igual regla strict. |
| `gofmt`/`vet` | corren en CI y en el gate | Sin excepciones por plataforma. |

Cada exclusión de herramienta está cubierta por `internal/testenv`
(NOV-65): en CI (`REGULUS_STRICT_ENV=1`) omitir es fallar.

## Laboratorio protegido

Runner estándar de GitHub ≠ enforcement: Smart App Control no está
aplicado ahí. La validación bajo audit/enforcement requiere VM o
self-hosted programado (ver procedimiento en `docs/windows-diagnosis.md`).
Última evidencia fechada: eventos CodeIntegrity 3077/3033/3118 del
2026-11-14 en estación de desarrollo (bloqueos de `*.test.exe` en
Temp, Policy ID `0283ac0f`, Enterprise signing) — audit, no enforcement.
Nunca desactivar protecciones ni confundir audit con enforcement.
