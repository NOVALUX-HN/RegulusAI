# Cross-Agent Learning (F12.6 / NOV-101)

Compartir conocimiento útil entre agentes **sin compartir conversaciones
indiscriminadamente ni convertir hipótesis en hechos** — tipos de memoria
con procedencia, recuperación por relevancia con ACLs, invalidación
gobernada y métricas de calidad.

> La hipótesis nunca es un hecho: se etiqueta explícita (`hipótesis — no
> es un hecho`), jamás entra en resultados `--verified-only`, y el
> Almanac **rehúsa** verificarla o aceptarla aunque cite evidencia.

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Sustrato | `almanac.Observation`: los 5 tipos nuevos + `decision`; revisiones por `topic_key`, `contentHash` + dedup vectorial (`dedup.go`), pins |
| Retiro | Relaciones `supersedes` / `invalidated_by` + estado `REJECTED` (`events.go`, `status.go`); `Contradictions()` vivo |
| Permisos | Vocabulario cerrado F11.8 (`mcp.ScopeMemoryRead`): proyecto/tarea exigen `memory:read`; privada lee autor; compartida es pública |
| Ranking | Orden determinista compartido (pinned, confianza, recencia — el mismo `sortHits` del canal léxico e híbrido) |
| Trazabilidad | `observability.RecordExec` (F11.1): cada mutación `learn` deja `run.start` + `task.end` + `run.end` |
| Secretos | `RedactObservation` (también cubre `Evidence` top-level) |

## Modelo

Seis tipos (`learn save --kind`): `verified-solution`, `pattern`,
`decision`, `error`, `workaround`, `hypothesis`. Cada registro guarda
**fuente** (`source`), **autor/agente** (`author`, requerido),
**fecha** (`saved_at`), **versión** (`version`), **confianza**
(autoevaluación `low|medium|high`), **alcance** (`scope`) y
**evidencia** (`evidence`: pruebas que respaldan, texto o rutas) más la
procedencia Almanac (`task`, `git_head`, `paths`, `receipts`,
`expires_at`).

Alcances: `private` (solo el autor), `task` (autor o misma `task` +
`memory:read`), `project` (`memory:read`, el legado sin scope lee
aquí), `shared` (público — el pool cross-agent).

Reglas de admisión (en código, no en prosa):

- `verified-solution` sin `evidence` **no entra** (`learn save`
  rehúsa; `memory save` no admite tipos learn).
- `hypothesis` **jamás** alcanza `VERIFIED`/`ACCEPTED` (`RecordStatus`
  rehúsa incluso con evidencia; el retrieval la excluye de
  `--verified-only` aunque un bundle importado restaure historial).
- Los tipos nuevos verifican **solo** contra `evidence` citada; las
  `decision` escuetas conservan su vía legada (juicio del revisor con
  nota) — el gate máquina no reescribe gobernanza F11.
- `decision` escueta (sin campos learn) sigue siendo legado: `memory
  save --type decision` no cambia; cualquier campo learn la somete a
  validación completa.

## Comandos

```sh
regulus learn save --kind <k> --title <t> --author <a> --scope <s> \
  [--confidence low|medium|high] [--evidence <e1,e2>] [--version <v>] \
  [--topic-key --domain --task --path <a,b> --receipt <r1,r2>] \
  [--source agent|human|core] [--expires <dur>] <content>

regulus learn search [terms] [--kind <k1,k2>] [--scope <s>] \
  [--verified-only] [--include-stale] [--include-superseded] \
  [--limit N] [--as <agent>] [--task <t>] [--scopes <s1,s2>]

regulus learn verify <id> --receipt <path> [--by <who>]
regulus learn invalidate <id> --reason <text> [--by <who>]
regulus learn resolve --winner <a> --loser <b> [--by <who>]
regulus learn metrics [--json]
```

`--scopes` por defecto es el compatible F11.8 (todo lo conocido);
`--scopes ""` explícito es el lockdown (solo `shared`). `search`
informa `N denied by ACL`. Las mutaciones (`save`, `verify`,
`invalidate`, `resolve`) registran traza F11.1 best-effort; `search` y
`metrics` son lectura y no registran.

## Recuperación, resolución e invalidación

- **Relevancia**: match AND sobre título+contenido, orden
  pinned → confianza → recencia, `Limit` al final. Lo retirado
  (superseded/invalidado/rechazado/caducado) oculta por defecto;
  `--include-stale` / `--include-superseded` lo devuelven.
- **Contradicciones**: `memory relate --relation conflicts_with`
  declara; `learn resolve` exige contradicción viva y registra
  `supersedes` atribuido (`by`) — el perdedor oculta, la historia
  conserva ambos.
- **Invalidación**: `learn invalidate` = transición `REJECTED` con
  actor+motivo; la auditoría es `status.jsonl` (`memory status <id>`
  la muestra). `prune` conserva el conocimiento verificado activo
  como conserva las lecciones; lo retirado purga como el resto.

## Métricas (`learn metrics`)

`total`, `verified` (vigente) / `verified_ever`, `hypotheses`
(`hypotheses_verified` debe ser 0), `obsolete` (rejected +
invalidated + superseded + expired), `contradictions` vivas, y:

- `precision` = verificadas vigentes / (verificadas + rechazadas +
  invalidadas) — 1 si nada decidido.
- `useful recall` = verificadas aún activas / verificadas alguna
  vez — 1 si nada verificado (lo verificado que se pudre baja aquí).
- `obsolescence` = retirado / total; `noise` = (rechazado +
  invalidado + lados de contradicciones vivas) / total — 0 en vacío.

## Límites

- `--limit 0` = sin techo. Sin scopes conocidos, `shared` es lo
  único visible (fail closed, nunca silencioso).
- Las decisiones legadas sin scope leen como `project`.
- Misma entrada ⇒ mismos bytes (la traza es metadato de registro).
