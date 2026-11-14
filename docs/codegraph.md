# CodeGraph opcional (NOV-54 / T35)

Decisión: sin daemon ni DB persistente. Exploración = ripgrep/find stdlib + `gopls` bajo demanda; CodeGraph solo opt-in read-only si el benchmark lo justifica.

Benchmark (reproducible, Go/TS):
- Navegación: ripgrep `grep -rn` + `go list` vs CodeGraph `symbols/refs`. Mide precisión (símbolo correcto), latencia, tokens (contexto leído), falsos positivos.
- Indexado: incremental, excluye secretos, generated/vendor, `.regulus/`, `binaries/`, carpetas grandes. Nunca commitea índices.
- Fallback: sin CodeGraph instalado → búsqueda textual transparente (misma capability `runtime`).

Coste/frescura/cobertura: documentar por repo (tamaño, lenguajes Go/TS, staleness por mtime). Hoy: no demuestra valor neto → no integrar.
