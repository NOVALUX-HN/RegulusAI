# archify-install — archify en opencode y gentle-shell

User authorization: "instala esto en opencode y en gentle-shell: https://github.com/tt-a1i/archify" (2026-10-30). Archify 3.2.0 (Apache-2.0): skill `archify/SKILL.md` + renderer `bin/archify.mjs`, diagramas como HTML standalone. Instalador sancionado: `npx skills add tt-a1i/archify -g`. Hallazgo de lead: `extensions/skill-registry.ts` de gentle-shell escanea `.agents/skills` y `.config/opencode/skills` (rutas compartidas con opencode) — un install global cubre ambas superficies. Read down to `## Log` before working.

## Acceptance

### Scenario: archify resolves in opencode

- Given a global install via the sanctioned installer
- When opencode starts (post-`/reload`) with the repo or anywhere
- Then the `archify` skill is listed/resolvable from the shared skills dir and `bin/archify.mjs --help` runs on Node >=18

### Scenario: archify resolves in gentle-shell

- Given the same installed skill files
- When the gentle-shell skill registry refreshes (`/skill-registry:refresh` or session start)
- Then the `archify` skill is indexed (SKILL.md frontmatter `name: archify` intact, renderer present)

### Scenario: no repo tree polluted

- Given the install runs with `-g` from outside any repo
- When it completes
- Then `git status --porcelain` in Regulus AI shows no archify files (only charts log updates if charted)

## Tasks

| ID | Spec | Work | Route |
|---|---|---|---|
| I1 | S1–S3 | run sanctioned installer global + verify both surfaces (SKILL.md, renderer --help, opencode skills dir, gentle-shell registry refresh note) | @regulus-worker |

## Log

- **L1**: Chart opened 2026-10-30. Evidence: API `tt-a1i/archify` v3.2.0; README install table (`~/.config/opencode/skills/`, `.agents/skills/`); local `~/.agents/skills/` exists (orca-cli, orchestration); gentle-shell registry scans shared paths.
- **L2**: I1 CLOSED + PROVE (post-restart resume) — installer already landed `~/.agents/skills/archify/` (SKILL.md `name: archify`, renderers, v3.2); `node bin/archify.mjs --help` exit 0; repo tree free of archify files. Refresh: opencode `/reload`, gentle-shell `/skill-registry:refresh`.
