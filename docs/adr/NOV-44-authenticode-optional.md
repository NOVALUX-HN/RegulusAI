# NOV-44 — Authenticode opcional, sin bloquear dev (T26)

Decisión: no firmar en dev/CI. Solo release público si hay aprobación gratuita, nunca autofirmado como confianza.

- SignPath Foundation u otro programa gratuito: elegibilidad no asumida, sin solicitud aprobada hoy.
- Alternativas: MSIX/Store según modelo de publicación; costos/límites por evaluar cuando se distribuya.
- Artefactos: firman solo PE finales (`bin/regulus.exe`, `binaries/regulus-windows-*.exe`). Temporales `go test` nunca son releases.
- Si se aprueba: solo en pipeline release, secretos fuera del repo, procedencia + verificación Authenticode/cadena/timestamp/publisher/hashes.
- `scripts/sign-local.ps1` = autofirmado CurrentUser `CN=NOVALUX-HN Local`, solo SmartScreen local. Nunca presentarlo como confianza universal Smart App Control.
- Ninguna clave en Git. CI/dev funcionan sin esto.
