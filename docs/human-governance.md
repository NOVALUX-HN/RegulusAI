# Gobernanza humana a nivel de misión (F13.6 / NOV-109)

Autonomía proporcional al riesgo, con autorizaciones vinculadas
exactamente a las acciones/cambios aprobados. La política por
defecto deniega: lo desconocido (kind, superficie, alcance, perfil)
se bloquea antes de actuar, y ningún perfil vuelve autónomo lo que
la matriz marca high/critical.

## De dónde sale cada pieza (sin duplicar)

- **Riesgo y gates**: `internal/verify` (vocabulario
  low/medium/high/critical, matriz riesgo→7 gates). Govern no
  redefine niveles ni inventa gates: `VerificationGates` cita los
  mismos que el motor de misión exige.
- **Permisos**: `internal/mcp` (F11.8). Los scopes concedidos salen
  del principal server-side (`PrincipalFor`: entorno + workspace
  resuelto, nunca flags); `authorize --mcp-tool` cruza además la
  capability policy sobre la misma acción. Govern suma la capa de
  misión, no reimplementa contención.
- **Separación de deberes + aprobación humana + pins**: `internal/evolve`
  `governed.go`/`validation.go` (proposer/evaluator/approver/deployer
  distintos, approver humano, commit/policy pins). Govern lo eleva a
  toda operación de misión; el ciclo sigue siendo dueño de su pin.
- **Traza**: F11.1 (`observability.RecordExec`, best-effort). El store
  `.regulus/governance/` cita ids, nunca duplica el exec log.

## La matriz

```
low      read reversible en superficie no sensible → autónomo en scopes explícitos (agrupable)
medium   write/execute reversible                 → autónomo solo en scopes explícitos del perfil
high     deploy/policy/permission/evaluator/security, superficies core/security/permissions/policy/evaluator/deploy, o irreversible → aprobación humana
critical irreversible gobernado o declarado       → aprobación humana ligada a hashes + gates completos
```

`regulus govern matrix` imprime la tabla que el motor aplica
(generada desde `ClassifyRisk`, no copiada a mano).

## Perfiles

| Perfil | Autónomo | Nota |
|---|---|---|
| `locked` | nada con efecto | todo write/execute+ denegado o con aprobación |
| `standard` (defecto) | low en scopes explícitos | rutina sin fricción, resto con humano |
| `wide` | low+medium en scopes explícitos | high/critical siempre con humano |

Configuración por proyecto/entorno: `regulus govern policy
--set-profile <p> --by <humano>`. Solo un humano cambia las reglas
(actores automatizados denegados) y el cambio queda en el audit log.
Sin fichero, rige `standard` deny-by-default.

## El flujo

```
authorize (¿autónomo? imprime riesgo + gates + veredicto MCP)
  → ALLOW: actúa (scopes explícitos, trazado)
  → NEEDS_APPROVAL: request → approve → check (antes de cada efecto)
  → DENY: bloqueado por defecto
```

`request` sella: resumen, diff-hash, plan/versión, riesgos, tests,
rollback, coste, alcance, más proposer/reviewer/executor (tres
identidades distintas) bajo la policy vigente con expiración
(defecto 24h, tope 168h). `approve` lo firma un humano fuera de los
tres deberes, ligado al hash exacto pedido (bait-and-switch
rechazado). `check` revalida antes de cada efecto: estado, expiración,
hash, techo de alcance y versión de policy. Cambio material invalida.

`pause`/`resume` (proposer o reviewer), `reject` (reviewer o humano
externo al build), `revoke` (proposer/reviewer/approver u operador
humano) con motivo auditado. Los aprobados revocados y los
rechazados detienen pendientes: el borde consulta `check` antes de
actuar, así que la revocación muerde de inmediato.

## Anti auto-aprobación

Cuatro deberes pairwise distintos (proposer, reviewer, approver,
executor) verificados en `RequestApproval` (los tres de construcción)
y en `Approve` (el firmante queda fuera de los tres). El approver es
humano (`IsHuman`: espejo del gate F11.6/F12.7 — `auto/bot/system…`
rechazados). La atribución de identidad es del operador (mismo modelo
de confianza que `evolve validation cycle-approve --by`): el nombre
llega por flag server-side, el gobernado nunca se auto-atribuye por
este camino. Ver `internal/govern/approval.go` (`Approve`, `Reject`
restringe quién juzga) y `adversarial_test.go`
(`TestAdversarialSelfApprovalMatrix`).

## Auditoría

- `.regulus/governance/approvals/apr_<id>.json`: el sello (creación
  exclusiva; `VerifyApproval` revalida vocabulario, binding y
  continuidad en cada lectura).
- `.regulus/governance/audit.jsonl`: append-only con cadena de hashes
  (`VerifyAudit` recomputa secuencia, enlace y contenido; corte,
  edición o reorden falla). `regulus govern audit [--id]` la reemite
  y verifica.

## No pedir por trivialidades

`Groupable` (low + no sensible) permite agrupar rutina segura; el
lote conserva la unión de scopes solo si cada miembro ya era
autónomo. Todo lo demás pasa por `request` una vez, no una vez por
operación.

## Comandos

```
regulus govern matrix [--json]
regulus govern authorize --kind <k> --surface <s> [--irreversible] [--scope <a,b>] [--profile <p>] [--mcp-tool <t> --mcp-arg <k=v,...>] [--json]
regulus govern request --kind <k> --surface <s> [--irreversible] --summary <t> --scope <a,b> --risks <a,b> --rollback <t> --cost <t> [--diff <sha>] [--plan <v>] [--tests <a,b>] [--mission <m>] --proposer <p> --reviewer <r> --executor <e> [--ttl-hours N] [--json]
regulus govern approve --id <apr> --by <human> --artifact <sha> --reason <t> [--json]
regulus govern check --id <apr> --artifact <sha> [--scope <a,b>] [--json]
regulus govern pause|resume|reject|revoke --id <apr> --by <actor> --reason <t>
regulus govern show --id <apr> [--json] | regulus govern list [--state <s>] [--json]
regulus govern audit [--id <apr>] [--json]
regulus govern policy [--show] [--set-profile <p> --by <human>] [--json]
```

Códigos: `authorize` 0 solo si autónomo (y MCP sin denegar); 1 si
requiere aprobación o deniega. `check` 0 VALID / 1 INVALID. Mutaciones
2 en uso, 1 en rechazo.
