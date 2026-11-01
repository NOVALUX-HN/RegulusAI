# Dashboard — harness vivo siempre encendido

Página local read-only en puerto fijo para ver lo importante del harness,
lo guardado y cómo aprende el Almanac. Abrirla y entenderla no requiere
leer código: cada sección dice qué significa en lenguaje simple.

## Abrir el panel

```bash
regulus serve --port=17817        # default 127.0.0.1:17817
# abrir http://localhost:17817/ (también vale /dashboard)
```

- Solo GET + loopback por defecto (`--bind` para cambiar, bajo tu riesgo:
  si bindeas fuera de loopback el servidor avisa por stderr que los datos
  del proyecto quedan expuestos a esa red).
- Errores con el mismo envelope que MCP (`regulus.http/v1` pineado).
- Auto-refresh cada 30s, sin dependencias (cero JS externo; la UI viene
  embebida en el binario).

## Arranque automático (las 3 plataformas)

Windows (PowerShell):

```powershell
powershell -NoProfile -File scripts/install-dashboard-autostart.ps1
# quita con -Uninstall
```

Crea `shell:startup\Regulus Dashboard.lnk` → `bin\regulus serve --port=17817`.
Tras reiniciar, abre `http://localhost:17817/`. Para probar sin reiniciar,
doble clic al acceso directo o corre `regulus serve` a mano.

Linux (systemd user, con fallback a cron):

```bash
bash scripts/install-dashboard-autostart.bash
# quita con --uninstall
```

Escribe `~/.config/systemd/system/regulus-dashboard.service`
(`ExecStart=<repo>/bin/regulus serve --port=17817 --bind=127.0.0.1`,
idempotente: re-ejecutar actualiza) e intenta `systemctl --user enable
--now`. Sin systemd imprime una línea `@reboot ...` para `crontab -e`.

macOS (LaunchAgent):

```bash
bash scripts/install-dashboard-autostart.bash
# quita con --uninstall
```

Escribe `~/Library/LaunchAgents/ai.regulus.dashboard.plist`
(`RunAtLoad`, binario + `serve --port=17817 --bind=127.0.0.1` del repo) e
intenta `launchctl load -w`. Sin `launchctl` imprime el comando manual.

Opciones del script POSIX: `--port=N`, `--repo=DIR`, `--bind=ADDR`,
`--uninstall`. Verificado: escribe la unit en Linux, el plist en macOS
(XML válido), avisa con el comando manual cuando no hay gestor de
servicios, e instalar/desinstalar dos veces seguidas deja un solo fichero
o ninguno. En cualquier plataforma el plan B siempre funciona:

```bash
regulus serve --port=17817
```

## Guía por sección (qué muestra cada parte)

Cabecera — estado del proyecto y salud del sistema. Lee `/v1/status`
(salida de `regulus status`: bien si `exit` es 0) y `/v1/version`, más
`/v1/metrics` (tareas, comprobaciones superadas de `receipts_pass` /
`receipts_total` y mejoras usadas hoy `evolution_used` / `evolution_cap`).
El desplegable «Ver estado completo» muestra el stdout tal cual. Todo
texto libre llega ya redactado (D3).

Resumen por áreas — 5 tarjetas con número + explicación simple:

- Guardado: todo lo conservado (`/v1/almanac/summary`, campo `total`).
- Evidencia: comprobantes de verificación (`/v1/receipts`, `total`).
- Memoria: recuerdos anotados (`/v1/observations`, `total`).
- Aprendizaje: lecciones confirmadas + por confirmar (`/v1/lessons`).
- Coste: movimientos del presupuesto (`/v1/budget`, `total`).

De un vistazo — 4 mini gráficos de barras CSS puros (sin JS externo):

- Recuerdos por tipo (`almanac/summary.by_type`).
- Lecciones confirmadas frente a por confirmar (`lessons_admitted` frente
  a `candidates_pending`).
- Presupuesto usado por tipo (agrupando `budget.items[].kind`).
- Comprobaciones por resultado (agrupando `receipts[].receipt.verdict`).

Evidencia — comprobantes de verificación. Cada fila es una vez que
Regulus comprobó que el trabajo estaba bien: tarea, prueba (`gate`),
resultado y fecha. Fuente: `GET /v1/receipts?limit=&offset=&task=`,
10 por página con Anterior/Siguiente.

Memoria — notas y decisiones. Lo que Regulus anotó para no olvidarlo.
Buscador de texto (`q` en título/contenido) + filtro por tipo. Fuente:
`GET /v1/observations?q=&type=&task=&limit=&offset=`, 10 por página.

Aprendizaje — lecciones. Lo ya confirmado (`Confirmada`) y lo que cree
haber aprendido pero está por confirmar (`Por confirmar`). Filtro por
estado. Fuente: `GET /v1/lessons?status=admitted|candidate|all&limit=`,
10 por página, texto truncado a 140 caracteres en la tabla.

Sesiones — el trabajo paso a paso. Cada sesión es una racha de trabajo
seguida con su nº de pasos; «Ver pasos» lista qué hizo Regulus en cada
paso, en orden (llamada, herramienta, resumen). Fuentes:
`GET /v1/traces?limit=&offset=` (sesiones) y
`GET /v1/traces?session=&limit=&offset=` (entradas), 10 por página.

Coste — presupuesto de mejora. Cada fila es un movimiento del presupuesto
que Regulus usa para mejorar solo: fecha, tipo, tarea y resultado.
Fuente: `GET /v1/budget?task=&limit=&offset=`, 10 por página.

Pie — se actualiza solo cada 30 segundos + botón «Actualizar ahora» que
recarga sin perder la página de cada tabla.

## Rutas

- `/` UI: cabecera de estado + tarjetas por área + gráficos + tablas (D2).
  `/dashboard` es un alias de `/`.
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

## Garantías (qué NO llega nunca al navegador)

- Redacción real: todo texto libre (stdout/stderr de `status`, `version`,
  `contract` y los 10 detalles de error de las colecciones) pasa por
  `RedactText` antes de serializarse (D1 + D3).
- CSP restrictiva + nosniff: el HTML se sirve con
  `Content-Security-Policy: default-src 'self'; script-src 'self'
  'unsafe-inline'; style-src 'self' 'unsafe-inline'; ...` (solo el inline
  propio) y `X-Content-Type-Options: nosniff`; la UI pinta todo dato con
  `textContent`, nunca `innerHTML` (D2 + D3).
- Topes: colecciones `limit` 1..200 y agregados sin texto libre, para que
  el panel nunca pida un volcado sin acotar (D1 + D3).
- Loopback por defecto con aviso: `regulus serve` escucha en `127.0.0.1`
  salvo `--bind` explícito, y bindear fuera de loopback escribe un aviso
  en stderr (D3).

## Evidencia en vivo (panel corriendo, 2026-11-01)

Cómo reproducir: `regulus serve --port=17817` (en Linux/macOS el mismo
comando; el binario de este repo en `bin/regulus`) y pedir cada ruta con
`curl`. Transcripciones reales recortadas a lo esencial:

```jsonc
// GET /v1/version -> cabecera "versión"
{"ok":true,"data":{"version":"regulus 0.10.0\n","contract":"regulus.contract/v1\n"},"contract":"regulus.http/v1"}

// GET /v1/almanac/summary -> tarjeta Guardado + gráfico "Recuerdos por tipo"
{"ok":true,"data":{"total":66,"by_type":{"decision":2,"discovery":2,"lesson":2,"session-note":60},
"lessons_admitted":2,"candidates_pending":0,"index_status":"stale"},"contract":"regulus.http/v1"}

// GET /v1/metrics -> cabecera "salud" (tareas, comprobaciones, evolution hoy)
{"metrics":{"tasks":101,"receipts_pass":1416,"receipts_total":1454,...},
 "evolution_used":0,"evolution_cap":10}

// GET /v1/receipts?limit=1 -> tabla Evidencia + gráfico "Comprobaciones por resultado"
{"total":1454,...,"items":[{"task":"almanac-g1-g4/M1",
 "receipt":{"gate":"build","verdict":"pass",...}}]}

// GET /v1/observations?limit=1 -> tabla Memoria (66 recuerdos)
{"total":66,...,"items":[{"type":"decision","title":"v0.2 almanac complete",...}]}

// GET /v1/lessons?status=all&limit=1 -> tabla Aprendizaje (2 lecciones)
{"total":2,...,"items":[{"Task":"regulus-ai-v1/T2","Status":"admitted",
 "Lesson":"write compiling stubs plus the full test suite first, ..."}]}

// GET /v1/traces?limit=1 -> tabla Sesiones (138 sesiones)
{"total":138,...,"items":[{"session":"ses_e6a61812effegohCEynLhQ0OOT","entries":44}]}

// GET /v1/budget?limit=1 -> tabla Coste (24 movimientos)
{"total":24,...,"items":[{"kind":"reflect","task":"regulus-ai-v1/T2",
 "outcome":"candidate-admitted",...}]}
```

Cabeceras y errores verificados contra el panel en marcha:

```text
GET / -> 200, 20347 bytes, contiene cards-grid
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'; ...
X-Content-Type-Options: nosniff
GET /dashboard -> 200 (alias de /)
POST /v1/receipts -> 405 {"code":-32600,"message":"invalid request: method not allowed, want GET (got POST)"}
GET /v1/receipts?limit=9999 -> 400 {"code":-32602,"message":"receipts: limit must be an integer 1..200"}
regulus serve --bind=0.0.0.0 -> stderr: "warning: regulus serve binds a non-loopback
address (0.0.0.0): the read-only dashboard exposes project data to that network; ..."
```

Nota honesta sobre capturas: en este entorno no hay navegador ni motor
headless disponible (sin chromium/chrome/edge/firefox/playwright; solo
`curl.exe`/HTTP), así que no hay PNGs del panel. En su lugar, esta
sección trae transcripciones reales del panel sirviendo este mismo repo
(ver comandos arriba): mismos números que pintan las tarjetas, tablas y
gráficos, mismas cabeceras CSP y mismos errores 405/400. La UI no tiene
dependencias externas, así que lo que devuelve `GET /` + estos JSON es
exactamente lo que ve quien abre `http://localhost:17817/`.
