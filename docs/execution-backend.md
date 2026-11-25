# Execution backend: verifying work on a host that blocks fresh binaries

## Symptom

`go build` succeeds and running the result fails:

```
fork/exec ...\pxexec.test.exe: Una directiva de Control de aplicaciones bloqueó este archivo.
```

Or, when launching the product itself:

```
$ ./bin/regulus.exe doctor
Permission denied
```

Compilation works. Execution never does. `go vet` and `gofmt` still work, because they run the signed toolchain from `GOROOT` and never launch a fresh PE.

## Cause

Windows application control in enforcement mode — Smart App Control or WDAC. On the affected host:

```
HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy
  VerifiedAndReputablePolicyState = 1        # 0=off 1=enforce 2=evaluation
Policy ID: {0283ac0f-fff1-49ae-ada1-8a933130cad6}   # Smart App Control
```

The CodeIntegrity operational log records event **3077** for each refusal.

This is not a code defect and it cannot be fixed from Regulus: every binary we link is new, so it has neither a trusted publisher signature nor Intelligent Security Graph reputation. Nothing in the repository can grant trust to its own build output.

## What does NOT work

| Attempt | Why it fails |
|---|---|
| Authenticode with a self-signed certificate | Smart App Control ignores untrusted roots |
| Authenticode with a public CA | each `go test` binary is new, so signing cannot keep up |
| Running from a different directory | the refusal is signature/reputation based, not path based |
| `go test -exec "wsl.exe -d Ubuntu -e"` | WSL receives the Windows path unconverted: `execvpe(C:\...\pxexec.test) failed` |
| `-gcflags=all=-trimpath=<win>=><linux>` | the repository path contains a space and the flag splitter breaks the mapping |

## What does work: run the work in the distro

Execution moves to WSL2, where no Windows policy applies. Compilation happens there too, so `runtime.Caller` records real Linux paths and subprocesses behave as CI expects.

Two entry points, both administrative-free:

### Verify the test suite

```bash
scripts/verify-wsl.sh                    # go test -short ./...
scripts/verify-wsl.sh ./internal/pxexec/ # any go test arguments
scripts/verify-wsl.sh --gates --node     # full toolset
```

### Run any Regulus command

```bash
scripts/regulus-wsl.sh doctor
scripts/regulus-wsl.sh status
scripts/regulus-wsl.sh gate run --quick --yes <task>
```

The launcher builds `cmd/regulus` for `linux/amd64` into `$HOME/.regulus/bin` (rebuilding only when sources changed) and executes the command inside the distro, from the same working tree the host sees through `/mnt/<drive>/`.

## Measured results on the affected host

| Approach | Packages green |
|---|---|
| Native Windows | 0 / 52 |
| Cross-compile + `go test -exec` | 36 / 52 |
| Go natively in WSL2 | 50 / 52 |
| + `gremlins` | 51 / 52 |
| + `node` | 52 / 52 |

`regulus gate run --quick --yes demo/T1` → **6/6 gates passed, verdict PASS**.
Full suite runs in roughly 8 minutes; targeted packages take seconds.

## Cost and reversibility

Everything installed lives in the distro's `$HOME`:

```
~/.local/go        Go toolchain
~/.local/node      Node (agent corpus fixtures)
~/go/bin           gremlins and other `go install` tools
~/.regulus/bin     the Linux regulus binary
```

No Administrator, no registry change, no Windows policy change. Revert:

```bash
wsl -d Ubuntu -e bash -lc 'rm -rf ~/.local/go ~/.local/node ~/go ~/.regulus'
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `REGULUS_WSL_DISTRO` | `Ubuntu` | distro used for execution |
| `REGULUS_WSL_GO` | `1.27.3` | Go version bootstrapped on first run |

## Design rule for future Go code

Anything that spawns a process must go through `internal/pxexec`, never `os/exec`. The suite enforces this: `TestNoRawProcessLaunch` and `TestEveryProductionSpawnHidesWindow` walk `internal/` and fail on a raw launch. `internal/execbackend` follows the same rule — its runner calls `pxexec.Run`, so console hiding, the minimal environment and the `BLOCKED` classification are not re-implemented.

## Known limits

* The sandbox is for local verification. CI remains the delivery authority.
* The 9p mount is slow for builds; the launcher mitigates by writing the binary inside the distro, but source reads still cross the mount.
* Gates that pass here are evidence of Linux behaviour. They do not prove the affected Windows host can execute anything.
