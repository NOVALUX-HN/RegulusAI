# Regulus Trust Foundation (Phase 1)

How Regulus proves what it evaluated, under which conditions, and with what
authority. Decisions behind this design: `docs/adr/T1-01` … `T1-04`.

## 1. The candidate has an identity

`gate run <task>` writes two things under `.regulus/evidence/<task>/`:

- one receipt per executed gate (append-only, hash-pinned stdout/stderr);
- one `manifest-<runid>.json`: the reproducible identity of the evaluated
  candidate — git HEAD + diff hash, modified/untracked inventories, gate
  config fingerprint, tool versions, environment-names hash (never values),
  applied policy, reproducibility counts, and links to every receipt.

A HEAD alone never identifies the content: read the manifest. If git is
absent the manifest says `available: false` instead of failing.

## 2. Receipts can be checked

`regulus evidence verify [--task <task>]` recomputes every hash, checks
versions and manifest links:

| Outcome | Meaning |
|---|---|
| `N checked, M passed` | sound (legacy receipts noted separately) |
| `FAIL <file>: stdout_hash_mismatch` | content changed after writing → quarantined to `corrupt/` |
| `FAIL …: manifest_missing / manifest_unlisted` | receipt sound, link broken → reported, file stays |
| `FAIL …: unsupported_schema` | newer writer → fails closed, file stays |

Recovery is re-running the gate. Nothing is ever auto-repaired or deleted.

## 3. One verdict authority

States: `PASS`, `FAIL`, `BLOCKED`, `INCONCLUSIVE`. Reasons distinguish
*deterministic_failure* (exit/metric) from *policy_violation* (refused
before running), *insufficient_evidence* (ran without recordable evidence),
and *infrastructure_failure* (timeout, spawn, resource cap). `gate run`
prints the final `verdict: STATUS (reason)` line. Guardians and agents may
dispute a verdict — the dispute is recorded, the core verdict stands.

## 4. Commands run under policy

`regulus.json` accepts an optional `execution` block (all defaults safe):

```json
"execution": {
  "max_timeout_sec": 3600,
  "allow_destructive": false,
  "allowlist": ["go", "bun", "git"],
  "env_unset": ["AWS_SECRET_ACCESS_KEY"],
  "docker_image": "golang:1.27"
}
```

Plus per gate: `"timeout_sec"`, `"reproducibility": "exact|statistical"`,
`"isolate": "docker"`. Destructive patterns (root wipes, mkfs,
`of=/dev/*`, fork bombs) refuse without `allow_destructive`; `gate run
--dry-run` prints the resolved plan and executes nothing. Docker is
opt-in: without a daemon the gate runs host-contained and says so in the
receipt. Host permissions remain the real boundary — this policy is the
auditable backstop.

## 5. Reproducibility, honestly

The manifest records what's needed to re-run (commands, config, versions,
inputs, output hashes, durations, CPU time) and counts gates by
reproducibility annotation. `unknown` is the default and a valid answer:
unclaimed reproducibility is reported, not assumed. Exact resource
accounting (peak RSS, cgroups) is out of scope — wall + CPU time are
recorded per gate.
