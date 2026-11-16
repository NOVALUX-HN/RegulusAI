# Release reproducible, mantenimiento y documentación canónica (F11.9 / NOV-95)

Cierra F11 con distribución verificable y documentación coherente con el
sistema real. No declara soporte sin journey ni evidencia: lo no probado
se lista como gap honesto, nunca como capacidad.

> Alcance de este documento: procedimiento de release para el operador,
> auditoría de workflows, evidencia de reproducibilidad local,
> auditoría de dependencias y reconciliación de claims. Es el índice
> canónico: enlaza la documentación de detalle, no la reescribe.

## 1. Procedimiento de release (para el operador — el tag y la publicación los hace el usuario)

Precondición: `develop` en verde (CI gauntlet + windows-native), CHANGELOG
con entrada `## [X.Y.Z] - DATE` y versiones alineadas (`package.json`,
`versioninfo.json` Fixed y String, `cmd/regulus/regulus.manifest`,
head de `CHANGELOG.md`, línea `Status:` del README — lo impone el gate
de deriva documental).

1. **Tag.** Desde `main` al hacer merge (nunca en `develop`):
   `git tag vX.Y.Z && git push origin vX.Y.Z`.
   Solo `refs/tags/v*` publica; un `workflow_dispatch` manual solo
   compila (artefactos dry-run) y nunca crea ni sobrescribe un Release.
2. **Builds.** `.github/workflows/release.yml` compila la matriz de
   6 targets con `-trimpath` y `-ldflags "-s -w -X main.version=X.Y.Z"`:
   linux/darwin/windows × amd64/arm64. Windows genera primero los
   recursos de versión con goversioninfo (falla cerrado: sin `|| true`).
3. **Checksums.** El job `release` genera `checksums.txt` (SHA-256) sobre
   los 6 binarios y lo publica junto a ellos.
4. **Attestations.** Cada binario lleva attestación de procedencia de
   compilación SLSA (`actions/attest-build-provenance`), verificable
   independientemente del checksum.
5. **Publicación.** Release de GitHub con notas extraídas de la entrada
   `## [X.Y.Z]` del CHANGELOG. La extracción es por prefijo (los heads
   llevan sufijo de fecha) y falla cerrado si la entrada no existe:
   una versión sin documentar nunca publica con notas vacías.
6. **Instalación limpia.** En una máquina/entorno sin estado previo,
   descargar el Release, verificar (paso 7) y seguir
   `docs/installation.md` (canales build + install-devin + launcher).
   El job `verify-published` hace exactamente esto en CI sobre
   ubuntu-24.04 y windows-2022: descarga, `sha256sum -c checksums.txt`,
   `contract`, `doctor`, journey mínimo (init → gate → close) y
   comprobación de que el `.exe` queda `NotSigned` (firma opcional,
   ver ADR NOV-44).

## 2. Verificación independiente (sin confiar en el CI)

Con `gh` autenticado y el tag ya publicado:

```bash
TAG=vX.Y.Z; rm -rf /tmp/relverify; mkdir -p /tmp/relverify; cd /tmp/relverify
gh release download "$TAG" --pattern 'regulus-*' --pattern 'checksums.txt'
sha256sum -c checksums.txt
gh attestation verify regulus-linux-amd64 --repo NOVALUX-HN/RegulusAI
chmod +x regulus-linux-amd64
./regulus-linux-amd64 contract   # regulus.contract/v1
./regulus-linux-amd64 doctor
```

El `gh attestation verify` comprueba la procedencia SLSA contra el
origen del repo; `sha256sum -c` comprueba integridad de los 6
artefactos. Ambos deben pasar; si uno falla, el Release no se consume.

## 3. Auditoría de workflows (verificado en este cambio)

Todos los workflows con `permissions: contents: read` por defecto y
escalado mínimo por job; toda acción de terceros pineada por SHA
completo con su tag en comentario; todo pin de herramienta Go exacto.

| Workflow | Permisos | SHAs / pines | Attestations | Veredicto |
|---|---|---|---|---|
| `release.yml` build | `contents: read` + `id-token`/`attestations`/`actions: write` (solo este job) | checkout, setup-go, attest-build-provenance, upload-artifact por SHA; goversioninfo exacto | procedencia por binario | REPARADO (3 fallos cerrados, §4) |
| `release.yml` release | `contents: write` + `actions: read`, solo en `refs/tags/v*` | download-artifact, action-gh-release por SHA | n/a (hereda procedencia de build) | REPARADO (guard de tag + notas) |
| `release.yml` verify-published | `contents: read` | release-downloader por SHA | verifica, no emite | OK (deferral documentado: reemplazar por `gh release` es cambio aparte con pruebas) |
| `ci.yml` gauntlet | `contents: read` + `actions: write` (subida de receipts) | checkout, setup-go, setup-bun por SHA; gremlins, govulncheck, actionlint exactos | n/a | OK (upload-artifact se queda en v4 a propósito: v5 también declara node20; ver comentario en el workflow) |
| `ci.yml` windows-native / protected-audit | `contents: read` + `actions: write` (evidencia) | checkout, setup-go, upload-artifact por SHA; gremlins exacto | n/a | OK |
| `ci.yml` traceability / secret-scan | heredan `contents: read` (suficiente: checkout + lectura) | checkout, setup-go por SHA; gitleaks exacto + `.gitleaks.toml` con allowlist solo de fixtures de test | n/a | OK |
| `fuzz.yml` | `contents: read` | checkout, setup-go por SHA | n/a | OK (6 superficies, 60 s semanales) |
| `host-conformity.yml` | `contents: read` + `actions: write` (evidencia) | checkout, setup-go por SHA; host pineado `opencode-ai@1.19.5` | n/a | OK (probe siempre verde + journey solo con secreto; nunca etiqueta probe como journey) |
| `mutation-weekly.yml` | `contents: read` + `actions: write` (logs) | checkout, setup-go, upload-artifact por SHA; gremlins exacto | n/a | OK (umbral de eficacia 70) |
| `perf.yml` | `contents: read` + `actions: write` (reporte) | checkout, setup-go por SHA | n/a | OK (compara contra `bench/perf-baseline.json`, falla cerrado) |

`actionlint` sobre los workflows tocados: limpio.
Dependabot (`.github/dependabot.yml`, semanal: github-actions, gomod,
bun) mantiene frescos los SHAs y los pines de herramientas.

## 4. Reparaciones aplicadas en este cambio (F11.9)

1. **Notas del CHANGELOG que siempre salían vacías.** El `awk` usaba
   igualdad de línea (`$0==v`) pero los heads llevan sufijo de fecha
   (`## [X.Y.Z] - DATE`): ninguna versión coincidía jamás y el Release
   publicaba con notas vacías. Cambiado a coincidencia por prefijo y
   verificado localmente: `0.15.0` extrae su sección, una versión
   inexistente queda vacía.
2. **Guardián de notas vacías.** `test -s release-notes.md`: una versión
   sin entrada en el CHANGELOG falla el job en vez de publicar vacío.
3. **Publicación solo en tags.** `release` lleva
   `if: startsWith(github.ref, 'refs/tags/v')`; `verify-published`
   cuelga de `release`, así que un `workflow_dispatch` manual solo
   compila (dry-run) y nunca crea ni toca un Release.
4. **goversioninfo falla cerrado.** Eliminados los `|| true`: si la
   generación de recursos de versión falla, el build de Windows falla
   en vez de distribuir binarios silenciosamente sin versionar.

## 5. Evidencia de reproducibilidad local (2026-11-16, sin publicar)

Compilación local replicando el workflow (mismo `-trimpath` y
`-ldflags`, versión `0.15.0`, sin tag): 6/6 targets en verde.

| Artefacto | SHA-256 |
|---|---|
| regulus-linux-amd64 | `06a7c2fb01071f34735fb39f6736b87ce6443361bcafc313e575f962cdf35d6f` |
| regulus-linux-arm64 | `f1df9d52799725498d15eed6ec266612d2bdbfe401759bc2a618c1c9b0ad4d45` |
| regulus-darwin-amd64 | `0635b64c25f99c8550ab1f7dc3b6f063724d783c80572fc894f71305698842ac` |
| regulus-darwin-arm64 | `d05e25963d9b030a0ccbe6e308d3e5bf6c117f193512524b78f9abee8482be94` |
| regulus-windows-amd64.exe | `87d4f0b9f85008d055c75808aaae6f4cd60c1a2754593818a7d3a67a6f5eb687` |
| regulus-windows-arm64.exe | `d11df7403631448abce198d38a0156eba8b79d405b5b8fa02bfcce822cfde27e` |

Verificación independiente sobre esos artefactos: `sha256sum -c`
6/6 OK; `contract` → `regulus.contract/v1`; `doctor` en verde
(versión coincide con `package.json`, OpenCode V2 soportado);
journey mínimo (init smoke → gate → close) PASS con receipts.
Nota: son hashes de verificación local, no del Release (el Release
lo construye el CI sobre el tag del usuario).

## 6. Auditoría de dependencias

Módulo Go sin dependencias externas (sin `go.sum` por invariante):
nada que actualizar. Herramientas y hosts pineados por versión exacta:

| Dependencia | Pineado | Último conocido (2026-11-16) | Estado |
|---|---|---|---|
| gremlins | v0.6.0 | v0.6.0 | Resuelta (al día) |
| govulncheck (x/vuln) | v1.9.0 | v1.9.0 | Resuelta (al día) |
| actionlint | v1.7.12 | v1.7.12 | Resuelta (al día) |
| gitleaks | v8.30.1 | v8.30.1 | Resuelta (al día) |
| goversioninfo | v1.4.0 | v1.7.0 | **Pendiente justificada**: recurso crítico del Release Windows; subir de versión sin prueba de compatibilidad (cambios de flags/recursos entre v1.4 y v1.7) rompería la garantía de §5. Se difiere como cambio aparte con prueba, igual que los deferrals de NOV-83. Seguimiento vía Dependabot semanal. |
| @opencode/plugin | 2.2.0 | 2.3.0 | **Pendiente justificada**: fuera de la superficie de este cambio; actualizar exige prueba de compatibilidad (`bun install --frozen-lockfile` + `tsc --noEmit` + `bun test`) y pertenece al release del usuario, no a este cierre. |
| @types/bun | 1.4.4 | 1.4.4 | Resuelta (al día) |
| typescript | 7.0.2 | 7.0.2 | Resuelta (al día) |
| opencode-ai (host-conformity) | 1.19.5 | verificada 2026-11-16 | Resuelta |
| codex CLI | 0.169.0 | verificada 2026-11-16 | Resuelta |
| claude-code (`@anthropic-ai/claude-code` sin pin en la guía) | sin pin | — | **Gap honesto**: sin versión fijada la instalación no es reproducible byte a byte; `bench host-journeys` lo informa como no-ejecutado, nunca pass. Pendiente: fijar versión verificada. |

Ninguna actualización se aplicó sin prueba; lo pendiente queda
justificado arriba con su condición de aceptación.

## 7. Documentación alineada con el código (F11.9 no reescribe: reconcilia)

| Área | Fuente canónica | Estado |
|---|---|---|
| Arquitectura | `docs/architecture.md` (Go 1.27, boundary TS/Go, boundary rule) | Alineada con el árbol actual |
| Contratos | `docs/contracts.md` (plugin↔core, MCP, wire protocol con refuse explícito) | Alineada; `contract` verificado en §5 |
| Hosts | `docs/conformity-matrix.md` + `docs/host-usage.md` + `docs/devin.md` | Alineada; matriz F11.7 con evidencia fechada 2026-11-16 (nota: estados `no-ejecutado` por falta de credenciales/binario, nunca pass) |
| Agentes | `docs/agents.md` (guardianes read-only + crew) + `docs/agent-evals.md` + `docs/agent-lab.md` | Alineada con F11.2/F11.7 |
| Eventos | `docs/trace-schema.md` (F11.1) + `docs/observability.md` (Phase 8) | Alineada; los spans se proyectan read-only, el schema versionado es la fuente |
| Benchmarks | `docs/perf-baselines.md` + `docs/perf-diagnosis.md` + `docs/eval-tiers.md` + `bench/perf-baseline.json` | Alineada con F11.3/F11.4; `perf.yml` compara semanalmente y falla cerrado |
| Operación | `docs/installation.md` + `docs/troubleshooting.md` + `docs/trust.md` + `docs/windows*.md` | Alineada salvo gap conocido abajo |
| Decisiones | `docs/adr/` (incl. NOV-44 firma opcional, NOV-21 trust local, NOV-23 receipts) | Enlazados desde las áreas que gobiernan |
| PRD | `docs/PRD.md` | **Histórico (H10)**: registro fundacional congelado F0–F5, excluido del gate de deriva; el estado actual vive en código, tests y la matriz de conformidad |

**Gap honesto (deriva no cubierta por el gate):** `docs/installation.md`
cita versión `0.13.0` mientras `package.json`, `versioninfo.json`, el
manifiesto y el README ya van en `0.15.0`. El gate de deriva solo
compara esas cinco fuentes entre sí, así que no lo detecta. Queda
pendiente para el release del usuario (fuera de la superficie de este
cambio); no se declara alineación donde no existe.

## 8. Reconciliación de claims de soporte y Done (matriz F11.7 + Linear)

Regla: una capacidad se declara **probada** solo con journey ejecutado
y evidencia enlazada; sin ejecución es **no-ejecutada**, sin soporte
conocido **no-soportada**, sin datos **desconocida**. Quinta palabra
distinta = exit 1 (`ValidateCapabilitiesAttributable`).

- OpenCode / Claude Code / Codex: fixture journeys E2E repetibles
  (install → doctor → context → spec → gate → close) con evidencia
  2026-11-16 enlazada en `docs/conformity-matrix.md`. Mínimo "al menos
  2 hosts E2E repetibles en CI": OpenCode + Codex.
- Agent-leg con modelo real: credencial-gated (`HOST_API_KEY`/login);
  sin credencial el job reporta no-ejecutado, nunca pass.
- Devin: estrategia realista documentada (pack `.devin/` + sesión
  Desktop/Cloud + evidencia importada), clase `strategy`: nunca
  ejecuta en CI, nunca se declara probada.
- Windows: probado en CI `windows-native`; Smart App Control real solo
  en lab (auditoría, no enforcement). Authenticode: `NotSigned`
  mientras la firma sea opcional (ADR NOV-44).
- Presupuesto de incertidumbre declarado en la matriz (timeouts sin
  reintentos, latencia no medida, `doctor` advisory que puede
  enmascarar fallos, sin tests ACL/merge MCP real): se documenta,
  no se oculta.

Nada en este documento declara soporte sin journey/evidencia. Los
estados NOV-87..93 están Done con evidencia citada en sus issues; este
cierre (NOV-95) queda verificable localmente (§5) y pendiente de
Release real + CI logs por el operador (§1–§2).

## 9. Aceptación F11.9 (mapeo a evidencia)

- Release verificable desde entorno limpio: procedimiento §1 + job
  `verify-published` (CI) + verificación local §5. Release real y tag:
  los ejecuta el usuario.
- Checksums/attestations comprobados independientemente: §2
  (`sha256sum -c` + `gh attestation verify`); local §5.
- Dependencias resueltas o justificadas: §6 (5 al día, 3 pendientes
  justificadas con condición de aceptación).
- Docs alineadas con código: §7 (7 áreas + ADRs + PRD histórico +
  1 gap honesto declarado).
- Sin soporte declarado sin journey/evidencia: §8.
