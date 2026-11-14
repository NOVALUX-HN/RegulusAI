// Package adversarial pins the versioned robustness suite (NOV-41).
// catalog.json is the attack catalog; corpus/ holds one fixed seed per entry.
// Every scenario runs against the real code, deterministic and hermetic.
// Rule: every new finding adds one catalog entry + one corpus seed + one test.
// No network, no model, no fuzzing: fixed seeds only.
package adversarial
