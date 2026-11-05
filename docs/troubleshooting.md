# Troubleshooting

## `/regulus-yolo` (or editing opencode.json) doesn't take effect immediately

Opencode reads its config once at startup: flipping YOLO (or any
permission) needs a refresh. Run `/reload` in the TUI (no full
restart), or restart opencode. The `regulus yolo` command always
prints this reminder after `on`/`off`.

`regulus yolo on` flips both `opencode.json` and the worker agent's
`bash "*"` frontmatter (`.opencode/agents/regulus-worker.md`) to
`allow`, keeping every destructive deny denied; `off` restores both
to `ask`. Guardians stay read-only (`deny`) and the primary agent is
never touched — so after `on` + `/reload`, neither primary nor worker
asks for normal bash/edit, while deletes stay blocked.

## Terminal windows flash open and shut on Windows whenever regulus runs

The opencode background server has no console on Windows, so every
spawn of the regulus binary (each tool call, each trace record, each
context hook) used to pop a visible terminal window. Fixed in two
places: the plugin spawns with `windowsHide: true`, and the Go code
marks its children via `internal/executil.HideWindow` (doctor probe,
gate runner, project-name lookup). If flashes return, a spawn site is
missing the marking — grep for `exec.Command` and route it through
`executil`.

## Windows marca el EXE como desconocido

Si Windows muestra el EXE como de publicador desconocido, o
SmartScreen / Defender avisa al ejecutarlo, la causa es que los EXEs
se construyen localmente sin firma Authenticode: no hay ningún
certificado público incluido en el repo, así que
`Get-AuthenticodeSignature → NotSigned` es esperado y no indica por
sí solo un binario manipulado. Desde T1 los EXEs llevan identidad en
Propiedades (pestaña Detalles): CompanyName NOVALUX-HN y ProductName
Regulus AI — eso identifica el binario pero no equivale a una firma.

Cómo permitir (elige solo lo que necesites):

- Si el archivo trae Mark-of-the-Web (descargado o copiado con marca
  de internet), desbloquéalo:
  ```powershell
  Unblock-File bin/regulus.exe
  ```
- SmartScreen en ejecutables descargados: en el aviso pulsa
  "Más información → Ejecutar de todas formas", solo si el hash
  coincide con tu build.
- Exclusión de Defender solo para la carpeta del repo y solo si
  confías en tu propio build:
  ```powershell
  Add-MpPreference -ExclusionPath 'C:\ruta\a\tu\repo'
  ```
  Revisa la exclusión con `Get-MpPreference` y retírala cuando ya no
  la necesites.
- Alternativa sin EXE (no dispara SmartScreen/Defender porque no hay
  binario que reputar):
  ```bash
  go run ./cmd/regulus ...
  ```

Límite honesto: la firma completa verificable por Windows
(Authenticode `Signed` con cadena a una CA pública) requiere un
certificado de firma de código público, no incluido en este repo. El
VersionInfo (CompanyName / ProductName) identifica el origen en
Propiedades pero no equivale a firma y no convierte `NotSigned` en
`Signed`.

## Firmar los EXEs en tus PCs (autofirmado local)

Si `Get-AuthenticodeSignature` sigue en `NotSigned` (ver la sección
"Windows marca el EXE como desconocido" arriba), puedes firmarlos para
tus propias PCs con un certificado autofirmado local, sin comprar nada:

```powershell
powershell -File scripts/sign-local.ps1
```

Requiere el Windows SDK instalado (por `signtool`): si falta, el script
falla con un mensaje claro y no toca nada.

Alcance: vale solo en las PCs donde se instaló el cert. El script genera
el cert code-signing NOVALUX-HN en `CurrentUser\My` y lo instala en
`Root` + `TrustedPublisher` del usuario actual (`CurrentUser`, nunca
`LocalMachine`): en esas PCs el `Status` pasa a `Valid` (firmado y
confiable). En cualquier otra PC sin el cert, el EXE sigue sin ser
confiable.

Límite honesto: esto no es un publisher verificado global — eso requiere
un certificado de firma de código público (una CA pública), no incluido
en este repo. La firma local quita el aviso en tus PCs, no ante el mundo.

Re-ejecutable sin miedo: el script es idempotente — si el cert ya existe
y los EXEs ya están firmados con él, re-correrlo es no-op.

## `EUNKNOWN: unknown error, uv_spawn` (Windows bloquea el binario)

Si cualquier comando `regulus` (o el plugin de OpenCode) falla con
`EUNKNOWN: unknown error, uv_spawn` (errno -4094 bajo Bun; bajo Node el
launcher informa `code=UNKNOWN` con el mismo errno), la causa no es PATH,
cwd ni permisos de archivo: es la directiva de **Device Guard / App
Control (WDAC)** de la organización bloqueando el binario antes de
ejecutarlo. libuv no tiene errno para ese rechazo, por eso el mensaje es
opaco. Desde v0.12.1 el plugin y `bin/regulus.js` detectan esta firma y
la explican en el propio error en vez de dejar el `EUNKNOWN` pelado.

Cómo confirmarlo (elegí una):

- `cmd.exe /C '"<ruta>\bin\regulus.exe" doctor'` muestra el aviso
  "ha sido bloqueado por la directiva de Device Guard de su
  organización".
- Visor de eventos → `Microsoft-Windows-CodeIntegrity/Operational`,
  eventos **3033/3077** nombrando el binario y el policy ID.
- `Get-AuthenticodeSignature` da `NotSigned`: esperado en un build
  local, y por eso la policy a nivel Enterprise lo rechaza.

Lo que NO lo arregla: `Unblock-File` (eso saca Mark-of-the-Web, no una
policy WDAC), la firma autofirmada de `scripts/sign-local.ps1` (solo
sirve contra SmartScreen), mover el binario de carpeta, recompilar, ni
`go run` (cuando `go.exe` está bloqueado por la misma policy, como se
ve en los mismos eventos 3033/3077).

Salidas reales:

- Pedir a quien administre la policy una excepción por hash o firmante
  para las rutas de dev (adjuntá los eventos 3033/3077 con el policy ID).
- Correr dentro de WSL (el kernel Linux no está sujeto a la CI de
  Windows) con `binaries/regulus-linux-*`.
- Una máquina no gestionada por esa policy para builds locales.

## El plugin usa un `regulus` viejo del PATH (stale core)

Si el setup del plugin rechaza con `stale core: binary says X but
package.json says Y`, el problema no es tu checkout: los candidatos
locales fallaron (ver sección EUNKNOWN arriba) y el plugin llegó al
último recurso — `regulus` en PATH — que resultó ser un core de una
versión anterior. Ese core responde `contract` pero corre una
superficie vieja (p. ej. 0.8.1 no conoce `--limit/--budget` de `memory
context` y rompe el hook de compaction en silencio), por eso el plugin
lo rechaza en vez de adoptarlo.

Cómo confirmarlo: el propio mensaje nombra el binario y ambas
versiones. Verificá con `<ese-binario> doctor`: la primera línea dice
`WARNING — binary says X but package.json says Y`.

Arreglo: reconstruí el binario local (`npm run build`, que estampa
`binaries/` + `bin/regulus[.exe]`) y eliminá o actualizá el `regulus`
viejo del PATH (`~/bin`, `~/.regulus/bin`) para que no vuelva a ganar
por descarte. El aviso `WARNING — unstamped dev build` de un `go
build` a mano, en cambio, sigue aceptado a propósito: es flujo de dev
local, no skew de versiones.

## Por qué las alertas nombran a `bun.exe` y `go.exe` (no son los culpables)

Si Windows muestra alertas nombrando a `bun` o `go`, no están
marcados como malware (Defender no registra ninguna detección
contra ellos; `bun.exe` está firmado válido y ambos corren): aparecen
como el **proceso padre** de binarios sin firmar que la policy WDAC
bloquea al nacer. Cada `go test` compila ejecutables frescos en
`Temp` (o `GOTMPDIR`), cada test de yolo construye un `regulus` de
prueba, cada `gremlins` del gate de mutación copia el módulo a Temp
— y cada hijo bloqueado suma eventos 3033/3077 más un posible aviso
nombrando al padre. Verificable en Visor de eventos →
`Microsoft-Windows-CodeIntegrity/Operational`: el bloqueado es
siempre el hijo (`*.test.exe` en Temp), nunca `bun`/`go`.

Mitigación práctica (no elimina la policy, reduce el ruido):

```powershell
$env:GOCACHE = "$HOME/.gocache"
$env:GOTMPDIR = "$HOME/.gotmp"
$env:TMPDIR = "$HOME/.gotmp"
```

Con eso los binarios de compilación/test nacen fuera de `Temp`
(la zona más castigada). No es garantía: la policy también ha
bloqueado `.gotmp` a rachas. El gate de mutación (`gremlins`)
es el mayor generador de avisos por diseño (un build por mutante
en Temp): en máquinas gestionadas preferí `--profile quick` o WSL
para corridas largas. La salida real sigue siendo la excepción
ante IT (sección EUNKNOWN arriba).

Experimento controlado (2026-11-05): el MISMO `surface.test.exe`
recién compilado corrió bien desde `.gotmp`, `Temp` y un dir nuevo
— ni la ruta ni el hash deciden por sí solos; la aplicación es
intermitente (ciclos de policy/MDM, reputación en nube). Conclusión
operativa: cada `go test` con rebuild es un boleto de lotería;
`go test` con caché (`ok (cached)`) no genera ningún binario y por
tanto ningún aviso. Evitá `-count=1` en corridas rutinarias (solo
para re-verificación real) y preferí paquetes puntuales al suite
completo. El test de yolo (`bun test` que compila+ejecuta un
`regulus` fresco en Temp) genera avisos por construcción en estas
máquinas — si lo ves fallar con `unknown error`, es el bloqueo,
no el producto (el propio test ahora lo diagnostica).

## `regulus doctor` says "opencode binary not found on PATH"

The regulus binary can't find `opencode`. Install OpenCode V2:

```bash
# Official installer (Linux, macOS, WSL, git-bash on Windows)
curl -fsSL https://opencode.ai/v2/install | bash
```

On Windows, the installer adds `~/.opencode/bin` to your PATH via `.bashrc`.
Restart your terminal or run `source ~/.bashrc`.

## `regulus doctor` says "OpenCode V1 is not supported"

Regulus targets OpenCode V2 exclusively. If you have V1 installed, remove it
and install V2 with the command above.

## `regulus status` says "gremlins NOT found"

The mutation gate (layer 6) needs [gremlins](https://github.com/go-gremlins/gremlins):

```bash
go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
```

## `regulus status` says "no regulus.json — not configured"

Create a `regulus.json` in your project root. See the README quickstart for
a full example with all twelve gate layers.

## The gauntlet's mutation gate fails with timeouts

On Windows, gremlins copies the whole module per mutant — including
`node_modules` if present — and every mutant starves to timeout. The gate
runs over a clean `git archive HEAD` export to avoid this; if it still
times out, check that `git archive HEAD` works and that your repo isn't
too large.

## The plugin doesn't find the regulus binary

The plugin searches, in order (first match wins): 1. `$REGULUS_BIN`,
2. `bin/regulus.exe`, 3. `bin/regulus`,
4. `binaries/regulus-<platform>-<arch>[.exe]` — on Windows (`win32`)
the same slot probes both names, primary `regulus-win32-<arch>.exe`
first and alias `regulus-windows-<arch>.exe` second (the shipped dir
only carries `regulus-windows-*`, so the alias is what hits),
5. `~/.regulus/bin`, 6. `regulus` on PATH (last resort, never preferred).
`EUNKNOWN` on slots 2–3 is the WDAC block, not PATH/cwd — see the
`EUNKNOWN` section above, don't chase PATH/cwd ghosts. Landing on slot
6 with `stale core` means slots 1–5 failed — see the stale-core section
above. If none exists, build the stamped local binary with the
documented flow:

```bash
npm run build   # = scripts/build.bash → binaries/ + stamped bin/regulus[.exe]
```

A raw `go build -o bin/regulus ./cmd/regulus` also works but stays
unstamped by design — `regulus doctor` warns (`WARNING — unstamped dev
build`) until you rebuild with `npm run build`.

## `regulus memory search` shows "N commits behind"

This is the git-distance staleness annotation: the observation's provenance
records the git HEAD at save time, and `git rev-list --count <head>..HEAD`
measures how far you've moved since. Update the observation if the code has
changed.

## Surface edits are refused with "outside the harness surface whitelist"

Surface edits can only target: `.opencode/skills/`, `.opencode/agents/`,
`.opencode/plugins/`, `.opencode/discipline.md`,
`.opencode/tool-guidance.md`, `docs/`, and `scripts/` (except reserved
files below). Code, configs and everything else are outside the harness
surface by design.

Apply additionally requires a named guardian validator, appends
per-extension markers (`//` for TS, `<!-- -->` for Markdown, `#`
elsewhere), and never creates files: the target must already exist.

## Surface edits are refused with "is a reserved harness file"

`scripts/build.bash` sits inside the whitelist but is reserved: release
infrastructure changes by hand (a human decision), never as a side
effect of an adopted lesson. Edit it directly; no flag overrides this.

## `regulus reflect` or `probe contrast` says "evolution cap reached"

Two caps gate evolution: per day (`max_per_day`, default 10) and per
task (`max_per_task`, default half the daily cap). A refusal names which
one (`cap N reached` vs `task cap N reached`) and is recorded but free.
Wait for the next day, or raise the cap in `regulus.json`.

## `regulus memory prune` lists entries but removes nothing

That's the design: without `--yes` it's a dry run. Re-run the exact
same command with `--yes` appended to execute. Lessons and pinned
observations are immune and never listed.

## `regulus memory import` says "line N: ..." and imports nothing

The import is atomic: the first invalid line aborts the whole file so
a half-imported log is impossible. Fix the cited line (missing id,
unknown type, lesson without evidence) and re-run — already-imported
ids are skipped, so re-running is safe. Importing a bundle directory
(`regulus memory import .regulus-export`) validates every file before
writing anything, and its errors name the file too
(`relations.jsonl line 3: ...`); events pointing at unknown
observations (orphan pins/relations/status) skip with a warning on
stderr instead of failing.

## `regulus evolve propose` says "no evidence"

Proposing needs receipts: run the gauntlet for the task first
(`regulus gate run <task>`), then propose. An evidence gate with no
evidence refuses instead of inventing a lesson.

## New charts lack the `Domain:` line

`regulus chart init` scaffolds `Domain: <name>` at the top — set it to
the lesson domain (e.g. `go`, `docs`) so the context hook injects
that domain's lessons instead of the newest of all domains.

## `regulus memory reindex` — when and why

Semantic search needs the derived vector index
(`.regulus/almanac/retrieval.bin`), built from your observation log:

```bash
regulus memory reindex
```

Mutating through the CLI needs no manual reindex: `memory note`,
`save`, `import` and `prune --yes` rebuild the derived index
automatically (best-effort, silent on success), so `regulus memory
doctor` reports `index: fresh` straight after the mutation and `memory
search` runs hybrid with no stale warning. Reach for a manual reindex
only when the log changed behind the CLI's back — right after
upgrading to v2.0, after hand-editing `observations.jsonl`, or whenever
search or `suggest` complains the index is stale. The index pins the
log's sha256, so staleness is exact, not heuristic. You don't have to guess: `regulus status`
reports a `memory:` line (`fresh`, `stale`, `missing`, or nothing to
index yet) and `regulus memory doctor` an `index:` line, both naming
the repairing command. Reindexing is a pure function of the log:
twice on the same log yields a byte-identical file, and stdout prints
the pinned sha. On an empty log it refuses ("no observations log yet")
— save an observation first.

## `regulus memory search` warns "semantic index stale or missing — lexical only"

That is the designed degrade path, not an error: the index is missing,
stale (the log changed since the last reindex), or corrupt, so search
ran lexical-only and told you on stderr, with the repairing command.
Results are still correct,
just without vector recall. Fix it with `regulus memory reindex`
(`regulus status` shows the staleness beforehand on its `memory:` line,
so the warning never surprises). To
force the old path on purpose (no warning), pass `--lexical`:

```bash
regulus memory search --lexical <terms>
```

Fresh index + no flag = hybrid ranking, silently. Fresh index + flag =
lexical, silently. Stale/missing index without the flag = lexical +
warning on stderr (stdout matches the `--lexical` output exactly).

## `regulus memory save` refuses with "near-duplicate save (matches ... similarity ..."

Saves scoring above 0.92 cosine against a live observation are refused
instead of stored as silent near-duplicates. The message names the
matching observation id and its similarity. Two cases:

- It really is a new revision of the same thought: re-save with
  `--topic-key <existing-key>` to append it to that chain instead.
- It is a different thought the hash embedder confuses: reword the
  title/content until the similarity drops (unrelated observations in
  the eval fixture score 0.26 or lower, far below the bar).

Refused saves exit non-zero and leave the log untouched. `memory import`
is exempt (re-importing identical content must succeed), and
observations hidden by a `supersedes` relation are excluded from the
comparison.

When the refused save really is a different thought the hash embedder
confuses, raise the bar for one command instead of rewording around it:

```bash
REGULUS_DEDUP_THRESHOLD=0.99 regulus memory save --type discovery --title ... <content>
```

The override must parse to a number in [0,1]; anything else (unparsable,
NaN/Inf, out of range) is rejected with a stderr warning and the default
0.92 applies. Without the variable the 0.92 bar and the refusal message
above are exactly as documented.

## `regulus memory relate --suggest <id>` says "requires a fresh semantic index"

Suggestions ARE the vector channel — there is no lexical fallback, so a
missing or stale index refuses (with a `reindex` pointer) instead of
presenting keyword hits as similarities. Run `regulus memory reindex`
first. Notes on the output:

- One line per candidate: `<id> <cosine> <title>`, top-5 above 0.5.
- Suggestion-only: nothing is written. Confirm with a plain relate:
  `regulus memory relate --id <a> --relation related --to <b>`.
- The source itself and `supersedes`-hidden observations never appear.
  No candidates ("no suggestions above threshold") means nothing cleared
  the 0.5 floor, not an error.

## `.regulus/traces/` and `.regulus/evidence/` keep growing — retention policy

`.regulus/` is local runtime state, gitignored (`.gitignore`), never
committed. CI uploads gate receipts as an artifact with
`retention-days: 30` (`gauntlet.yml:77`); local runs are the audit
source the CI trail is rendered from.

There is no global `gc` command by design — evidence of closed
tasks must not be deleted without authorization. Prune per scope,
and only what you own:

```bash
# evidence: dry run by default (lists what would be removed, deletes
# nothing); --yes executes. --keep N keeps the N newest receipts
# (default 0); manifests (manifest-*.json) are never deleted and
# corrupt/ is never touched. A task marked CLOSED in charts/ refuses
# without --force plus explicit authorization:
regulus evidence prune --task <slug> [--keep N]
regulus evidence prune --task <slug> --keep 5 --yes
regulus evidence prune --task <closed-slug> --keep 5 --yes --force   # only with authorization

# traces: per-session or bulk rotation (default --keep 200 keeps the
# last 200 entries of each session). Files are never removed — prune
# rewrites via temp+rename only, with a per-session report:
regulus trace prune --session <session-id> [--keep N]
regulus trace prune --all [--keep N]
```

Sizes at last review (2026-10-31): 82 trace sessions,
62 evidence task dirs (821 receipts, 2 manifests). Nothing is deleted
automatically — the directories stay until their owners prune them.

## Toolchain installs (pinned)

The heavy gates need three binaries outside the Go module. The pin
source is CI (`.github/workflows/gauntlet.yml:31-35`); local installs
must match it exactly, otherwise `regulus status` and the gate report
a binary the CI would not have used:

```bash
go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
```

`bun` comes from the upstream installer (https://bun.sh), then the
project deps, exactly as CI does (`.github/workflows/gauntlet.yml:29-41`):

```bash
bun install
```

`gremlins` drives the layer-6 mutation gate, `govulncheck` the layer-8
vulncheck gate, `bun` the layer-10/11 TypeScript gates. If any is
missing from `PATH`, the gate refuses before layer 1 and its message
names the install above — install the missing binary and re-run.

## Gauntlet profiles: `quick` vs `full`

`gate run --profile NAME` runs required gates plus the profile's
members (`docs/gauntlet.md` §2, `internal/gates/gates.go:93-98`):

- `quick` — required gates only (fast feedback);
- `full` — everything (the default when no profile is given).

Required gates always run: neither profiles nor `when` conditions can
omit them. The `mutation` gate keeps `profiles: [full]`
(`regulus.json`) but stays `required: true`, so it runs under both
profiles — `quick` never silently skips mandatory proof. Any other
profile name must be declared by at least one gate, else the run
refuses rather than running a silent subset.

## Mutation runs on a hermetic HEAD export

Since T1 the layer-6 mutation gate runs gremlins over a clean export,
not over your working tree: the runner exports `git archive HEAD`
(tracked files only) into a fresh temp dir and runs the gate command
there, so untracked bloat such as `node_modules` never reaches
gremlins (the old Windows timeout cause, see "The gauntlet's mutation
gate fails with timeouts" above). Portable implementation (`os` +
`os/exec` + `archive/tar`, no POSIX `tar`/`mktemp`); the temp dir is
always removed.

Fallback: when the export fails (not a git checkout, `git` absent,
empty archive) the gate runs in-situ and the receipt carries the
warning instead of failing silently:

- clean run: `mutation: clean export via git archive HEAD (hermetic, no untracked files)`
- fallback: `warning: mutation without clean export (not a git checkout)`

Either way the verdict still comes from efficacy/exit facts — the note
only records which tree the facts came from. If you see the fallback
warning, check that `git archive HEAD` works in your checkout.

## Wontfix (with evidence — do not re-file)

- **No npm publish.** Dropped as unnecessary by owner decision:
  `CHANGELOG.md:331-333` (G3.8 — personal use, local build + plugin
  by path cover the workflow, CI builds from source) and
  `README.md:19-23` (quickstart is build-from-repo: `regulus-ai` is
  not published). Revisit only if the project ever goes public.
- **Plugin-16 vs MCP-12 gap (gap-4) is intentional.** The OpenCode
  agent surface is 16 tools; MCP deliberately exposes 12. The 4-gap
  (`evolve_propose`, `probe_contrast`, `probe_compose`, `skills_list`)
  is a Devin-side policy decision, not drift: `internal/cli/mcp_test.go:179-215`
  (`TestMCPIntentionalGapPinsPluginDelta`) pins the exact gap, and
  `docs/devin.md:133-137` lists what is deliberately NOT ported and
  why. Evolving the tool list means updating `docs/devin.md` too.
- **`bash "*"` allow is the yolo design, not a hole.**
  `docs/troubleshooting.md:3-15` (`/regulus-yolo` section): `regulus
  yolo on` flips worker bash to `allow` (with every destructive deny
  kept denied, guardians read-only), `off` restores `ask`. After `on`
  + `/reload`, normal bash/edit stops asking while deletes stay
  blocked — that is the documented behavior.
- **Unstamped binary warning is by design, message is OS-aware.**
  `docs/troubleshooting.md:72-78`: a raw `go build -o bin/regulus`
  stays unstamped on purpose — `regulus doctor` warns (`WARNING —
  unstamped dev build`) until you rebuild with `npm run build`
  (stamped `binaries/` + local `bin/regulus`).
