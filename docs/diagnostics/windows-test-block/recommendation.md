# F14.4 — Recomendación técnica y plan F15

Basada en F14.1 (`inventory.md`), F14.2 (`timeline.md` + `events/`) y F14.3
(`repro-matrix.md`). Sin implementar nada: esta es la decisión que F15 ejecuta.

## 1. Auditoría (hallazgo → archivo/línea)

- Runner con workaround: `scripts/test-windows.ps1` (sidecar `.regulus-test/`,
  `GOTMPDIR` externo, clasifica BLOCKED vs FAIL, exit 1 solo por fallos reales).
  Funciona pero es externo al harness: cada agente debe conocerlo.
- Diagnóstico previo: `docs/windows-diagnosis.md` (NOV-43, Policy `0283ac0f`,
  hipótesis Temp+reputación) — confirmado y refinado por F14.2/F14.3.
- Firma local WIP: `scripts/sign-dev.ps1` + `scripts/dev-wdac.ps1` — la firma sola
  NO desbloquea Device Guard (lo dice el propio script); la suplementaria por
  publisher está pendiente y requiere admin. No es vía de F15.
- Go config: `GOCACHE` local con FullControl, `GOTMPDIR` unset (Temp por sesión),
  `CGO_ENABLED=0` — nada que corregir para el bloqueo (es load-time, no build-time).
- Workflows: `.github/workflows/ci.yml` + `fuzz.yml`, `host-conformity.yml`,
  `perf.yml`, `release.yml` — job Windows existe (verificar matriz exacta queda
  para F15.1 al consolidar el runner).
- Gates: `go vet`/`go build` pasan siempre (no ejecutan); `go run` ejecuta pero no
  sustituye tests. Regla vigente: BLOCKED nunca es PASS.

## 2. Separación interno / externo

Corregible en Regulus (sin tocar el host): runner determinista que compile a
directorio controlado, ejecute, capture exit/stdout/duración y clasifique
passed/failed/blocked/skipped/not-run por paquete con JSON reproducible; documentar
el no-determinismo por binario fresco en vez de reintentar a ciegas.
Externo (fuera de nuestro control): política SAC administrada, reputación ISG por
hash, privilegios de firma/exclusión, laboratorio limpio, WSL2.

## 3. Matriz comparativa de alternativas

| Alternativa | Eficacia | Requisitos | Riesgos | Prueba de éxito |
|---|---|---|---|---|
| A. Runner in-repo determinista (recomendada) | Alta para diagnóstico y suite local | Solo código; sin privilegios | No elimina el bloqueo, lo clasifica | `bench mission`-style: JSON por paquete + exit≠0 ante blocked en modo full |
| B. Cert publisher + firma de launcher | Media (solo si la política lo reconoce) | Admin + cert legítimo/autorizado | Falsa seguridad si SAC no lo reconoce; coste gestión | Binario firmado corre 10/10 corridas con eventos 3077 ausentes |
| C. Host CI limpio separado | Alta (aislamiento total) | Infra + mantenimiento | Coste; deriva con el host afectado | Suite verde reproducible fuera del equipo |
| D. WSL2 para tests portables | Media (solo portable) | WSL2 | Nunca prueba Windows nativo; falsa confianza | Matriz que separa portable vs nativo |
| E. Reintentos del binario fresco | Baja (no-determinismo) | Ninguno | Enmascara el fallo; CI flaky | Descartada: F14.3 demuestra varianza por link |

Descartadas por falsas universales (S4.5): certificado autofirmado como bypass de
Smart App Control; exclusiones amplias; desactivar Defender/WDAC/AppLocker;
renombrar binarios o cambiar extensiones.

## 4. Recomendación principal y fallback

- **Principal**: A. Consolidar el runner en el repo (F15.1 = causa raíz en el flujo
  de ejecución propio; F15.2 = runner desatendido observable). Cero cambios al host,
  cero debilitamiento, rollback = conservar los scripts actuales.
- **Fallback**: C (host CI limpio) si la tasa de blocked local impide validar releases;
  D como complemento de cobertura portable con la matriz explícita.

## 5. Plan F15

| Orden | Tarea | Depende | Aceptación |
|---|---|---|---|
| 1 | F15.1: consolidar runner (compila→ejecuta→clasifica→JSON), tests del clasificador, docs, rollback | NOV-115 revisada | Causa raíz corregida en el flujo o bloqueo externo demostrado con precisión; tests + rollback |
| 2 | F15.2: runner desatendido nativo observable (veredictos por paquete, exit codes, sin interacción) | 1 + NOV-115 revisada | OpenCode lo corre E2E; bloqueado nunca es PASS |

Rollback: `scripts/test-windows.ps1` y docs actuales se conservan hasta que el
nuevo runner los reemplace con paridad demostrada.

## 6. Prerrequisitos humanos / admin

- Nada de F15 requiere al usuario salvo que se elija la vía B/C (cert o host):
  entonces se necesita aprobación administrativa explícita + justificación, y F15.1
  se detiene antes de cambiar nada (spec NOV-116.3).
- Revisión humana obligatoria de este F14 antes de iniciar F15 (gate del spec).

## 7. Referencias a evidencia

- `inventory.md` + `inventory.json` + `scripts/diag-windows.ps1` (F14.1).
- `timeline.md` + `events/codeintegrity-2026-11-17.xml` (3077/3089/3118/3033,
  PolicyID `{0283ac0f-fff1-49ae-ada1-8a933130cad6}`, SHA256, Unknown) (F14.2).
- `repro-matrix.md` (8 escenarios, discriminador padre/contexto, no-determinismo) (F14.3).
