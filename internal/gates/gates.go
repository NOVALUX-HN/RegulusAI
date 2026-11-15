package gates

// gauntlet config model, outcomes and validation (NOV-67 split; behavior unchanged).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Gate is one configured quality gate. A gate with a Metric is a metric
// gate: the command must exit 0 AND the extracted metric must satisfy the
// operator against the threshold (F2, PRD R-G2 layers 5–7).
type Gate struct {
	Name            string   `json:"name"`
	Layer           int      `json:"layer"`
	Command         string   `json:"command"`
	TimeoutSec      int      `json:"timeout_sec,omitempty"`
	Metric          *Metric  `json:"metric,omitempty"`
	Reproducibility string   `json:"reproducibility,omitempty"`
	Isolate         string   `json:"isolate,omitempty"`
	ID              string   `json:"id,omitempty"`
	Category        string   `json:"category,omitempty"`
	When            []string `json:"when,omitempty"`
	OnFail          string   `json:"on_fail,omitempty"`
	Required        *bool    `json:"required,omitempty"`
	Profiles        []string `json:"profiles,omitempty"`
}

// Failure policies: halt stops the gauntlet at the first failure (the
// historical behavior); advisory records the failure and continues.
const (
	OnFailHalt     = "halt"
	OnFailAdvisory = "advisory"
)

// Reserved profiles: quick runs required gates only, full runs everything
// (the default when no profile is given). Any other name must be declared
// by at least one gate's profiles list.
const (
	ProfileQuick = "quick"
	ProfileFull  = "full"
)

// EffectiveID returns the gate's stable identity: the explicit id, or a
// slug of the name when unset.
func (g Gate) EffectiveID() string {
	if strings.TrimSpace(g.ID) != "" {
		return strings.TrimSpace(g.ID)
	}
	return gateSlug(g.Name)
}

func gateSlug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "gate"
	}
	return b.String()
}

// EffectiveOnFail returns the gate's failure policy, defaulting to halt.
func (g Gate) EffectiveOnFail() string {
	if g.OnFail == OnFailAdvisory {
		return OnFailAdvisory
	}
	return OnFailHalt
}

// IsRequired reports whether the gate is mandatory. Gates are required
// unless explicitly opted out: mandatory policy can never be omitted by
// omission — neither profiles nor when-conditions skip a required gate.
func (g Gate) IsRequired() bool {
	return g.Required == nil || *g.Required
}

// Metric extracts a number from the gate command's stdout and compares it
// against a threshold.
type Metric struct {
	Extract   string  `json:"extract"` // ExtractLastLineFloat | ExtractLastLinePercent
	Threshold float64 `json:"threshold"`
	Operator  string  `json:"operator"` // ">=" | "<="
}

// Metric extraction modes.
const (
	ExtractLastLineFloat   = "last-line-float"
	ExtractLastLinePercent = "last-line-percent"
)

// Metric comparison operators.
const (
	OpAtLeast = ">="
	OpAtMost  = "<="
)

// EvolutionConfig caps the evolution loop (PRD R-E4): reflect and probe
// runs per calendar day. The ledger records every run, refused ones
// included. MaxPerTask optionally bounds one task's share of the daily
// budget (a glottonous task must not eat the whole day); when unset or
// non-positive it defaults to half the daily cap (minimum 1).
type EvolutionConfig struct {
	MaxPerDay  int `json:"max_per_day,omitempty"`
	MaxPerTask int `json:"max_per_task,omitempty"`
}

// Config is the project's regulus.json.
type Config struct {
	Gates     []Gate           `json:"gates"`
	Evolution *EvolutionConfig `json:"evolution,omitempty"`
	Execution *ExecutionConfig `json:"execution,omitempty"`
}

// DefaultTimeoutSec bounds one gate when no explicit timeout is configured.
const DefaultTimeoutSec = 600

// MaxOutputBytes caps one gate's captured output per stream (stdout,
// stderr). A gate that floods beyond the cap is killed: unbounded output
// is a resource-exhaustion vector (memory, receipt size, log spam), not
// evidence.
const MaxOutputBytes = 8 << 20

// Outcome is one executed gate's summary.
type Outcome struct {
	Name        string
	Layer       int
	Verdict     string
	DurationMS  int64
	Status      string
	Reason      string
	ReceiptPath string
	ReceiptID   string
	TimeoutSec  int
	Skipped     bool
	SkipReason  string
	Advisory    bool
}

// Result is one gauntlet run.
type Result struct {
	Executed   int
	Passed     int
	Failed     bool
	FailedGate string
	Outcomes   []Outcome
	Receipts   []string
	Status     string
	Reason     string
	Skipped    []string
}

// Load reads and validates the gates config at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("no regulus.json found at %s — create one with a gates array", path)
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("malformed regulus.json: %v", err)
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validate enforces the config contract: at least one gate, unique names and
// layers, layers >= 1, non-empty names and commands, plus the Phase 1
// execution policy fields (reproducibility, isolate, allowlist).
func validate(cfg Config) error {
	if len(cfg.Gates) == 0 {
		return errors.New("no gates configured")
	}
	seenLayer := map[int]string{}
	seenName := map[string]bool{}
	for i, g := range cfg.Gates {
		if err := validateGateBasics(i, g, seenLayer, seenName); err != nil {
			return err
		}
		if err := validateGateMetric(i, g); err != nil {
			return err
		}
		if err := validateGatePolicy(i, g); err != nil {
			return err
		}
		if err := validateGateConditions(i, g); err != nil {
			return err
		}
		if err := validateGateID(i, g); err != nil {
			return err
		}
	}
	if err := validateExecution(cfg); err != nil {
		return err
	}
	return nil
}
