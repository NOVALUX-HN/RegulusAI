# Security Policy (NOV-86 Paso 66)

## Report a vulnerability

**Do not open a public issue.** Report privately through
[GitHub Private Security Advisories](../../security/advisories/new)
for `NOVALUX-HN/RegulusAI` (`Security` tab → `Advisories` → `New draft
security advisory`). No security email is monitored; the advisory
channel is the only private intake.

Response targets (best-effort, pre-1.0): acknowledge within 5 business
days, triage within 10, fix for critical findings within 30–90 days.
We will keep the reporter updated on the advisory thread and credit
them in the release notes unless they decline.

## Scope

In scope (what we ship and sign):

- `regulus` CLI core (`internal/*`, `cmd/regulus`), plugin
  (`.opencode/plugins/regulus.ts`), read-only dashboard
  (`regulus serve`, `internal/httpapi`).
- Install and signing scripts (`scripts/build.*`,
  `install-devin.*`, `install-dashboard-autostart.*`,
  `sign-local.ps1`), `bin/regulus.js` + `bin/verify-hash.js`.
- Distribution: `binaries/`, `checksums.txt`, SLSA provenance and the
  `release.yml` workflow that produces them.

Out of scope: `node_modules/`, forks and downstream copies, third-party
actions/pins (report upstream), social-engineering and physical access.

## Supported versions

Only the latest `vX.Y.Z` tag on `main` is supported
(`package.json` version is the single source of truth; tags are never
cut on `develop`). Older tags receive no patches — upgrade first, then
re-test.

## Disclosure

Coordinated disclosure: we publish the advisory and the fix release
together once a fix is ready, never before. Do not disclose details
until the advisory is public.
