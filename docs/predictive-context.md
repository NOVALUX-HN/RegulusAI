# Contexto predictivo (F12.3 / NOV-98)

Entregar a cada agente el **contexto mínimo suficiente, pertinente y
actualizado** — inventariado, rankeado con razones, con duplicados,
obsolescencia, contradicciones y fuentes no confiables detectadas, en
capas imprescindible / bajo demanda / opcional, con tokens y calidad
medidos antes y después, respetando ACLs y secretos, con fallback
conservador.

> La selección **propone una entrega**, no ejecuta nada: `context select`
> no lanza ningún agente. El fallback conservador prefiere de más a
> omitir contexto crítico.

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Descriptor de tarea | `orchestrate.TaskDescriptor` (F12.1: dominio, lenguaje, complejidad, riesgo, restricciones, outputs, criterios), validado por `orchestrate.ValidateTaskDescriptor` |
| Medición de tokens | Proxy F11.5 (caracteres/4, como `trace.EstimateLocalTokens` sobre los campos guardados): desconocido nunca es cero, cada capa mide |
| Memoria con procedencia | `almanac.All` (fuente, dominio, caducidad `ExpiresAt`, receipts); lo importado (`SourceImport`) lee como no confiable, nunca se blanquea |
| Permisos | Vocabulario cerrado F11.8 (`mcp.Scope*`): la memoria exige `memory:read`; sin scope no hay entrega |
| Secretos | Marcador `[redacted]` (como `almanac.Redacted`): la estructura se conserva, el valor nunca viaja |
| Traza | `bench.TraceHintFor` + store F11.1 (`observability.RecordExec`, vía el mismo registro que `orchestrate decide`) |

CodeGraph **no** se usa aquí: el ranking es propio y explicable (la
spec lo reserva para F12.8 y solo con benchmark).

## Comandos

```sh
# Censo del corpus por kind, con avisos de acotación
regulus context inventory [--issues <issues.json>] [--no-memory] [--json]

# Seleccionar contexto para un descriptor F12.1 (exit 0 entrega,
# exit 1 rehúsa —corpus vacío o crítico omitido—, exit 2 uso)
regulus context select --domain <d> --lang <l> \
  --complexity <low|medium|high> --risk <low|medium|high|critical> \
  [--cap <a,b>] [--constraint <a,b>] [--output <a,b>] [--criteria <a,b>] \
  [--profile <agente>] [--scope <s,...>] [--affinity <d,...>] \
  [--budget-tokens N] [--max-age-days N] [--untrusted <src,...>] \
  [--issues <issues.json>] [--no-memory] [--dry-run] [--out <dir>] [--json]
```

`--issues` acepta un JSON estricto `[{id,title,body,critical,scope}]`
(acotado a 1MB/1000 entradas; lo hostil falla cerrado). Sin
`--dry-run` se registra la traza F11.1; `--dry-run` calcula e imprime
sin registrar nada.

## Cómo se rankea y por qué capas

1. **Inventario acotado**: fuentes `.go` (un fragmento `code` + un
   `symbol` por declaración top-level `func`/`type`), `go.mod`
   (`dependency` por require con su versión), `docs/*.md` (`doc`) y
   `docs/adr/*` (`adr`, siempre crítico), issues del fichero
   explícito y memoria del Almanac (con dominio, caducidad y
   confianza según procedencia). Cada fragmento lleva fuente,
   versión (git head o `worktree`) y fecha; sin ellas no se entrega.
   `MaxFiles`/`MaxFileBytes`/símbolos acotan; lo acotado avisa, no
   se trunca en silencio.
2. **ACL primero**: sin el scope exigido no hay entrega — el
   fragmento se excluye de todas las capas y cuenta en el informe
   (`denied`), nunca se rankea.
3. **Ranking explicable**: el descriptor F12.1 se pliega a términos
   distintos (dominio, lenguaje, restricciones, outputs, criterios,
   capabilities; las palabras vacías no puntúan). Puntuación =
   términos coincidentes / términos totales + 0.15 de afinidad si el
   dominio del fragmento coincide con el del perfil (tope 1). La
   razón nombra cada término coincidente (hasta 8, luego "+N más").
   Orden determinista: puntuación desc, desempate por id.
4. **Detección**: secretos se redactan antes de entregar;
   duplicados (sha256 del contenido normalizado) apuntan al
   canónico; inyecciones (`ignore … instructions`, `reveal …
   system prompt`, exfiltración — el contenido es DATO, nunca
   instrucción) van a cuarentena; obsoletos (caducidad vencida o
   edad sobre el horizonte, defecto 180 días) y fuentes no
   confiables degradan una capa; contradicciones declaradas se
   marcan en ambos lados (leer ambos antes de confiar, como los
   conflictos del Almanac).
5. **Capas**: crítico ⇒ siempre imprescindible (si la pertinencia
   no lo ganó, con flag `fallback-conservador`); puntuación ≥ 0.35
   ⇒ imprescindible; > 0 ⇒ bajo demanda; 0 ⇒ opcional. Cuarentena
   (duplicado/inyección) topa en opcional. Si nada alcanza
   imprescindible, se promociona lo mejor rankeado que no esté en
   cuarentena ni obsoleto; si nada cualifica, imprescindible queda
   honestamente vacía y avisa.
6. **Medición antes/después**: informe con tokens del corpus,
   denegados, visibles e imprescindibles, % ahorrado (visible →
   imprescindible), recall crítico (debe ser 1 — menos es un fallo
   del fallback, se avisa, nunca se oculta) y conteos por detector.
   El budget recorta de atrás adelante (opcional, luego bajo
   demanda); imprescindible nunca se recorta — si lo excede, avisa.

## Límites

- `--budget-tokens 0` = sin techo; `--max-age-days 0` = horizonte
  de 180 días. Fechas desconocidas nunca leen como obsoletas.
- La memoria exige `memory:read` en el perfil; sin scopes solo se
  entrega lo público.
- Misma entrada ⇒ mismos bytes (la traza es metadato de registro;
  en `--dry-run` no se registra nada).
