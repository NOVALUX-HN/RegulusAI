# Trazabilidad de cierres (NOV-64 / T45)

## Línea base medida (F5–F7, develop)

24 issues cerrados en 4 commits multi-issue, un único autor de git y
cierres en ráfagas de segundos en Linear:

| Commit | Issues | Alcance declarado |
|--------|--------|-------------------|
| `2fe01cf` | NOV-43, NOV-45, NOV-46, NOV-44, NOV-47 (F5, 5) | Runner Windows + CI + docs |
| `001c7ac` | NOV-48…NOV-55 (F6, 8) | Contrato + 3 adaptadores + registry + ODD/RDD + CodeGraph + conformidad |
| `3de37af` | NOV-56, NOV-57, NOV-58 (F7, 3) | Protocolo + manifest + motor |
| `0169c23` | NOV-59, NOV-60, NOV-61 (F7, 3) | Evaluador + promoción + E2E |

Lección: "Done" quedó respaldado solo por el comentario de quien
implementó. Desde F8, el cierre exige evidencia enlazada.

## Política

- Un issue implica al menos un commit que lo cite (`NOV-xx` en subject o cuerpo).
- Commits multi-issue solo justificados: trailer `Justified: <motivo>` en el mensaje o flag `--justified` al cerrar. Sin justificación, el cierre falla nombrando el commit.
- Formato de cierre en Linear: `## T<n> NOV-xx — Implementado (<qué>)` + `**Qué se hizo:**` + `**Resultado:**` (comandos, exit codes, commits, receipts).
- Verificación del harness al cerrar (`task close --issue`): commits existentes, diff dentro del Scope del Chart (NOV-32), receipts referenciados existentes y verificados (flujo `CheckClose` existente).
- P0: veredicto independiente registrado como receipt (evaluación de un guardian distinto del implementador, o humano). Reutiliza `guardian-independence` (T3-02) y `verdict-authority` (T1-01): el revisor igual al implementador se rechaza; sin `--implementer` no se puede juzgar independencia.

## Excepciones legítimas

Hotfix (post-fix con issue creado justo después), merge (agrupa commits ya enlazados), solo-docs (sin receipts: el gate docs-drift es la evidencia). Ninguna abre atajos silenciosos: quedan registradas con su motivo.
