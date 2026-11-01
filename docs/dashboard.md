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

## Vista (D2)

La UI (`/` y `/dashboard`, embebida en el binario, sin dependencias
externas, auto-refresh cada 30s) está pensada para no-técnicos: todo el
texto dinámico se pinta con `textContent`, nunca `innerHTML` con datos.

- **Cabecera**: título en lenguaje simple, estado del proyecto (bien /
  revisar según `/v1/status` + versión), salud del sistema (tareas,
  comprobaciones superadas y mejoras usadas hoy según `/v1/metrics`) y
  un desplegable con la salida completa del estado.
- **Resumen por áreas**: 5 tarjetas con número + explicación simple:
  Guardado (todo lo conservado, `/v1/almanac/summary`), Evidencia
  (comprobantes, `/v1/receipts`), Memoria (recuerdos,
  `/v1/observations`), Aprendizaje (lecciones, `/v1/lessons`) y Coste
  (presupuesto, `/v1/budget`).
- **De un vistazo**: 4 mini gráficos de barras CSS puros (sin JS
  externo): recuerdos por tipo, lecciones confirmadas frente a por
  confirmar, presupuesto usado por tipo y comprobaciones por resultado.
- **Tablas paginadas** (10 por página, botones Anterior/Siguiente con
  `?limit&offset` de D1): Evidencia (tarea/prueba/resultado/fecha),
  Memoria (título/tipo/fecha + buscador `q` y filtro por tipo),
  Aprendizaje (tarea/estado/lección + filtro confirmada/por confirmar),
  Sesiones (sesión/nº pasos + «Ver pasos» que lista los pasos en orden) y
  Coste (fecha/tipo/tarea/resultado).

## Rutas

- `/` UI: cabecera de estado + tarjetas por área + gráficos + tablas (D2).
- `GET /v1/status`, `/v1/version` (las de siempre; textos libres redactados).
- `GET /v1/metrics`: `Summarize` + evolution hoy vs cap.
- `GET /v1/almanac/summary`: conteos por tipo, admitidas, pendientes, índice.
- `GET /v1/receipts?task=&limit=&offset=`: receipts del gauntlet, redactados, paginados.
- `GET /v1/observations?q=&task=&type=&limit=&offset=`: observaciones del Almanac con búsqueda (`q` en título/contenido), redactadas, paginadas.
- `GET /v1/lessons?domain=&status=&task=&q=&limit=&offset=`: lecciones admitidas + candidatas (`status=admitted|candidate|all`), redactadas, paginadas.
- `GET /v1/traces?session=&task=&limit=&offset=`: sin `?session=` lista sesiones con conteo; con `?session=` lista entradas en orden de llamada, redactadas, paginadas.
- `GET /v1/budget?task=&limit=&offset=`: historial del ledger de evolution, redactado, paginado.
- Paginación: `limit` default 50, máximo 200; `offset` default 0. Valores fuera de rango → 400 con el envelope compartido.
- Solo GET: cualquier otro verbo en rutas conocidas → 405 con el envelope compartido.

## Arranque automático (Windows)

```powershell
powershell -NoProfile -File scripts/install-dashboard-autostart.ps1
# quita con -Uninstall
```

Crea `shell:startup\Regulus Dashboard.lnk` → `bin\regulus.exe serve --port=17817`.
Tras reiniciar, abre `http://localhost:17817/`. Para probar sin reiniciar,
doble clic al acceso directo o corre `regulus serve` a mano.
