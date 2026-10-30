# Yolo fix — yolo on silencia de verdad

User authorization: "arregla el yolo de Regulus porque sigue pidiendo permisos para todo". Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: yolo on silencia primary y worker sin abrir deletes

- Given `opencode.json` con yolo off y worker en ask
- When `regulus yolo on` corre + `/reload`
- Then ni primary ni worker piden permiso para bash/edit normales; los denies destructivos siguen denied

### Scenario: yolo off restaura ask

- Given yolo on
- When `regulus yolo off` corre + `/reload`
- Then bash/edit vuelven a ask y `yolo status` lo reporta

### Scenario: guardians siguen read-only

- Given yolo on
- When se inspeccionan guardians
- Then `edit: deny` y `bash deny` intactos; ningún guardian edita código

### Scenario: gauntlet verde tras el fix

- Given el fix de yolo
- When corre `gate run yolo-fix/F1`
- Then 9/9 pass con receipts; yolo-permissions test actualizado pinneando el nuevo contrato

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| F1 | yolo on/off/status | `yolo.go` propaga a worker + tests + docs reload | @regulus-worker |

## Log

- L1: Chart opened desde "sigue pidiendo permisos para todo". Causa scout: yolo on solo toca opencode.json global; worker ask y guardians deny por frontmatter prevalecen por subagente; sin /reload ni el global aplica.
- L2: F1 CLOSED — Task F1 "`yolo.go` propaga a worker + tests + docs reload". yolo on flipa worker allow + denies + orden, off restaura ask, guardians intactos deny; yolo-fix/F1 9/9 rec_18e350dc...-rec_18e350fc... Chart CERRADO. Requiere /reload.
