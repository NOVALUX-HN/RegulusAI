// Package runtime is the agent-runtime boundary of Regulus Core (Phase 7).
//
// Core concentrates policies, verification, evidence, receipts, memory,
// evolution and decision assessment, and depends on nothing but the Go
// standard library plus regulus-ai/internal packages. The mapping:
//
//	policies            -> internal/gates (policy, profiles, execution)
//	verification        -> internal/evidence (+verify), internal/verdict
//	evidence            -> internal/evidence, internal/manifest
//	receipts            -> internal/receipts
//	memory              -> internal/almanac
//	evolution           -> internal/evolve, internal/budget
//	decision assessment -> internal/decisions
//
// Runtimes (OpenCode today; others tomorrow) live outside this boundary
// and talk to the core only through the Adapter contract: lifecycle
// events, task context, agent identity, tool capabilities, permission
// boundaries, execution metadata, context exchange, result reporting and
// error propagation. No agent SDK is imported anywhere under
// internal/ — boundary_test.go pins that.
package runtime
