# Dashboard — harness vivo siempre encendido

Página local read-only en puerto fijo para ver lo importante del harness,
lo guardado y cómo aprende el Almanac.

## Uso

```bash
regulus serve --port=17817        # default 127.0.0.1:17817
# abrir http://localhost:17817/
```

- Solo GET + loopback por defecto (`--bind` para cambiar, bajo tu riesgo).
- Errores con el mismo envelope que MCP (`regulus.http/v1` pineado).
- Auto-refresh cada 30s, sin dependencias.

## Rutas

- `/` UI: Harness / Guardado / Aprendizaje.
- `GET /v1/status`, `/v1/version` (las de siempre).
- `GET /v1/metrics`: `Summarize` + evolution hoy vs cap.
- `GET /v1/almanac/summary`: conteos por tipo, admitidas, pendientes, índice.

## Arranque automático (Windows)

```powershell
powershell -NoProfile -File scripts/install-dashboard-autostart.ps1
# quita con -Uninstall
```

Crea `shell:startup\Regulus Dashboard.lnk` → `bin\regulus.exe serve --port=17817`.
Tras reiniciar, abre `http://localhost:17817/`. Para probar sin reiniciar,
doble clic al acceso directo o corre `regulus serve` a mano.
