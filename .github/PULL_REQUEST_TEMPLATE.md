<!-- NOV-86 Paso 66: PR checklist mirrors docs/close-traceability.md. -->
## Linked issue

Fixes NOV-__ (link).

## Chart (if behavior changed)

`charts/<name>.md`, task `T<n>`.

## Evidence

- `regulus gate run <chart>/<task>` receipts: `rec_...`
- `regulus task close --issue NOV-xx ... --verify-only`: exit code
- Docs updated alongside the behavior (help text included):

## Scope confirmation

- [ ] Only the chart scope changed, or the change is justified below
- [ ] No secrets, credentials, or personal data in the diff
- [ ] Versioned evidence committed where the Paso requires it (manifest + sha256)

Justification (only if the diff leaves the chart scope):
