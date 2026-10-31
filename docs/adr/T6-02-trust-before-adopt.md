# T6-02 — Trust and approval enforced inside Adopt, no CLI in scope

## Status

Accepted.

## Context

S10 demands untrusted proposals plus no self-approval; S12 demands proof.
Two designs were possible: (a) checklist as a CLI pre-step before Adopt,
(b) checklist verified structurally by Adopt itself.

## Decision

(b). `Adopt` refuses unless the six-point `TrustCheck` verifies, the
domain check passes, the baseline gate holds and a human approver is
named. A CLI pre-step can be skipped or forgotten; a structural gate
cannot. Attestation happens while RUNNING (`AttestTrust`); verification
happens at adoption — a gap can be recorded but never adopted through.

Phase 6 ships library-level (`internal/evolve`) with no new CLI group:
the spec names pipeline, testing and acceptance, not commands. CLI wiring
(`regulus evolve experiment …`) is explicit future work, not silent scope.

## Consequences

Adoption is safe by construction even for programmatic callers. The
acceptance test (`TestEndToEndAcceptance`) is the proof: formulate,
isolate, compare, detect, reject the harmful, adopt the verified, revert.
