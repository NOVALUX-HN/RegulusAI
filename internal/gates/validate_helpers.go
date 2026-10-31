package gates

import (
	"fmt"
	"strings"
)

// validateGateBasics checks identity and ordering: non-empty name and
// command, layer >= 1, unique layers and names. It mutates the seen maps
// exactly as the monolithic validate did, preserving duplicate errors.
func validateGateBasics(i int, g Gate, seenLayer map[int]string, seenName map[string]bool) error {
	if g.Name == "" {
		return fmt.Errorf("gates[%d]: name must not be empty", i)
	}
	if g.Command == "" {
		return fmt.Errorf("gates[%d]: command must not be empty", i)
	}
	if g.Layer < 1 {
		return fmt.Errorf("gates[%d]: layer must be 1 or greater", i)
	}
	if prev, dup := seenLayer[g.Layer]; dup {
		return fmt.Errorf("duplicate layer %d: gates %q and %q", g.Layer, prev, g.Name)
	}
	seenLayer[g.Layer] = g.Name
	if seenName[g.Name] {
		return fmt.Errorf("duplicate gate name %q", g.Name)
	}
	seenName[g.Name] = true
	return nil
}

// validateGateMetric checks the metric operator and extract mode.
func validateGateMetric(i int, g Gate) error {
	if g.Metric == nil {
		return nil
	}
	switch g.Metric.Operator {
	case OpAtLeast, OpAtMost:
	default:
		return fmt.Errorf("gates[%d]: metric operator must be >= or <=", i)
	}
	switch g.Metric.Extract {
	case ExtractLastLineFloat, ExtractLastLinePercent:
	default:
		return fmt.Errorf("gates[%d]: metric extract must be last-line-float or last-line-percent", i)
	}
	return nil
}

// validateGatePolicy checks reproducibility, isolation and failure policy.
func validateGatePolicy(i int, g Gate) error {
	switch g.Reproducibility {
	case "", "exact", "statistical", "unknown":
	default:
		return fmt.Errorf("gates[%d]: reproducibility must be exact, statistical or unknown", i)
	}
	switch g.Isolate {
	case "", "docker":
	default:
		return fmt.Errorf("gates[%d]: isolate must be docker", i)
	}
	switch g.OnFail {
	case "", OnFailHalt, OnFailAdvisory:
	default:
		return fmt.Errorf("gates[%d]: on_fail must be halt or advisory", i)
	}
	return nil
}

// validateGateConditions checks profiles and when patterns are non-empty.
func validateGateConditions(i int, g Gate) error {
	for _, p := range g.Profiles {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("gates[%d]: profiles must not contain empty names", i)
		}
	}
	for _, w := range g.When {
		if strings.TrimSpace(w) == "" {
			return fmt.Errorf("gates[%d]: when must not contain empty patterns", i)
		}
	}
	return nil
}

// validateGateID checks the optional stable id charset [A-Za-z0-9_-]+.
func validateGateID(i int, g Gate) error {
	if strings.TrimSpace(g.ID) == "" {
		return nil
	}
	for _, r := range g.ID {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("gates[%d]: id must match [A-Za-z0-9_-]+", i)
		}
	}
	return nil
}

// validateExecution checks the execution allowlist entries are non-empty.
func validateExecution(cfg Config) error {
	if cfg.Execution == nil {
		return nil
	}
	for _, a := range cfg.Execution.Allowlist {
		if strings.TrimSpace(a) == "" {
			return fmt.Errorf("execution allowlist entries must not be empty")
		}
	}
	return nil
}
