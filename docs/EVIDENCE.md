# Local evidence — live-plus-gaps/L3 (NOT the CI receipts trail)

> Local audit copy for Chart `polish-gaps` S2. This is NOT the CI-owned page.
> CI regenerates `docs/EVIDENCE.md` on `main` only, from
> `regulus evidence list --task ci/push` after a green 12-layer gauntlet
> (`.github/workflows/gauntlet.yml:66-77`, gated on
> `success() && github.ref == 'refs/heads/main'`, committed as
> `github-actions[bot]` with `[skip ci]` so it cannot loop).
> This local file renders `bin/regulus evidence list --task live-plus-gaps/L3`
> at 2026-10-30T10:43-06:00 (America/Mexico_City), git HEAD `239623f`, on a
> Windows dev machine. It is expected to differ from CI and to be overwritten
> by the receipts-trail commit on the next green `main` run — that overwrite is
> by design, not a conflict. CI is untouched by this file (workflow not modified).
>
> Why only-main in CI: the trail must stay append-only and loop-free; PR runs
> still leave evidence via the `Upload gate receipts` artifact step
> (`path: .regulus/evidence/ci__push/`, `retention-days: 7`,
> `if-no-files-found: warn`), so nothing is lost by not committing on PRs.
>
> Local equivalent (no CI needed):
>
> ```powershell
> go build -o bin/regulus ./cmd/regulus
> bin/regulus evidence list --task live-plus-gaps/L3
> # single receipt detail:
> bin/regulus evidence show .regulus/evidence/live-plus-gaps__L3/<gate>-<ts>.json
> ```
>
> Source: `internal/cli/evidence.go` (`evidence list [--task <t>] | show <file>`,
> human audit surface over `.regulus/evidence/` receipts, never diffs).

Task `live-plus-gaps/L3`: 24 receipts = 12 gates x 2 consecutive green gauntlet
runs (2026-10-30, layers 1-12 incl. spec L9 + ts-typecheck L10 + ts-unit L11 +
gofmt L12), every line `pass exit=0`.

```
2026-10-30T16:21:49Z layer3 build        pass exit=0 641ms .regulus\evidence\live-plus-gaps__L3\build-1793377309765734200.json
2026-10-30T16:24:14Z layer3 build        pass exit=0 635ms .regulus\evidence\live-plus-gaps__L3\build-1793377454043348200.json
2026-10-30T16:22:15Z layer5 coverage-floor pass exit=0 674ms metric=83.80 >= 80.00 .regulus\evidence\live-plus-gaps__L3\coverage-floor-1793377335036088400.json
2026-10-30T16:24:39Z layer5 coverage-floor pass exit=0 671ms metric=83.80 >= 80.00 .regulus\evidence\live-plus-gaps__L3\coverage-floor-1793377479008761300.json
2026-10-30T16:22:59Z layer7 crap         pass exit=0 366ms .regulus\evidence\live-plus-gaps__L3\crap-1793377379053182900.json
2026-10-30T16:25:23Z layer7 crap         pass exit=0 371ms .regulus\evidence\live-plus-gaps__L3\crap-1793377523929790900.json
2026-10-30T16:23:04Z layer12 gofmt        pass exit=0 879ms .regulus\evidence\live-plus-gaps__L3\gofmt-1793377384246833000.json
2026-10-30T16:25:29Z layer12 gofmt        pass exit=0 861ms .regulus\evidence\live-plus-gaps__L3\gofmt-1793377529191969400.json
2026-10-30T16:21:49Z layer2 lint         pass exit=0 512ms .regulus\evidence\live-plus-gaps__L3\lint-1793377309253046100.json
2026-10-30T16:24:13Z layer2 lint         pass exit=0 514ms .regulus\evidence\live-plus-gaps__L3\lint-1793377453528788000.json
2026-10-30T16:22:15Z layer6 mutation     pass exit=0 43341ms .regulus\evidence\live-plus-gaps__L3\mutation-1793377335711329200.json
2026-10-30T16:24:39Z layer6 mutation     pass exit=0 44248ms .regulus\evidence\live-plus-gaps__L3\mutation-1793377479681147200.json
2026-10-30T16:23:02Z layer9 spec         pass exit=0 356ms .regulus\evidence\live-plus-gaps__L3\spec-1793377382106550600.json
2026-10-30T16:25:27Z layer9 spec         pass exit=0 348ms .regulus\evidence\live-plus-gaps__L3\spec-1793377527057729300.json
2026-10-30T16:23:02Z layer10 ts-typecheck pass exit=0 425ms .regulus\evidence\live-plus-gaps__L3\ts-typecheck-1793377382463170500.json
2026-10-30T16:25:27Z layer10 ts-typecheck pass exit=0 422ms .regulus\evidence\live-plus-gaps__L3\ts-typecheck-1793377527406814400.json
2026-10-30T16:23:02Z layer11 ts-unit-tests pass exit=0 1358ms .regulus\evidence\live-plus-gaps__L3\ts-unit-tests-1793377382888263500.json
2026-10-30T16:25:27Z layer11 ts-unit-tests pass exit=0 1360ms .regulus\evidence\live-plus-gaps__L3\ts-unit-tests-1793377527830541800.json
2026-10-30T16:21:48Z layer1 typecheck    pass exit=0 1073ms .regulus\evidence\live-plus-gaps__L3\typecheck-1793377308179469700.json
2026-10-30T16:24:12Z layer1 typecheck    pass exit=0 1076ms .regulus\evidence\live-plus-gaps__L3\typecheck-1793377452451652600.json
2026-10-30T16:21:50Z layer4 unit-tests   pass exit=0 24620ms .regulus\evidence\live-plus-gaps__L3\unit-tests-1793377310410968700.json
2026-10-30T16:24:14Z layer4 unit-tests   pass exit=0 24328ms .regulus\evidence\live-plus-gaps__L3\unit-tests-1793377454679380600.json
2026-10-30T16:22:59Z layer8 vulncheck    pass exit=0 2685ms .regulus\evidence\live-plus-gaps__L3\vulncheck-1793377379420200000.json
2026-10-30T16:25:24Z layer8 vulncheck    pass exit=0 2755ms .regulus\evidence\live-plus-gaps__L3\vulncheck-1793377524301359200.json
```
