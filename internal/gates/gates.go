// Package gates runs the layered quality gauntlet (PRD R-G2): gates
// execute in ascending layer order, cheap and binary first; a failing layer
// stops the gauntlet — no gate runs after a failure.
package gates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"regulus-ai/internal/executil"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/verdict"
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

// outputCapError reports a gate killed for exceeding the output cap.
type outputCapError struct{ bytes int64 }

func (e *outputCapError) Error() string {
	return fmt.Sprintf("output exceeded %d bytes cap", e.bytes)
}

// capWriter buffers up to limit bytes, discards the rest, and kills the
// process once on overflow. It never returns a write error: a failing
// writer would wedge the pipe copy and deadlock the wait.
type capWriter struct {
	buf      bytes.Buffer
	limit    int
	total    int64
	overflow bool
	once     sync.Once
	kill     func()
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.total += int64(len(p))
	if w.buf.Len() < w.limit {
		w.buf.Write(p[:min(len(p), w.limit-w.buf.Len())])
	}
	if w.total > int64(w.limit) && !w.overflow {
		w.overflow = true
		w.once.Do(func() {
			if w.kill != nil {
				w.kill()
			}
		})
	}
	return len(p), nil
}

// CmdRunner executes one shell command. The real one goes through a POSIX
// sh with a PowerShell/cmd fallback; tests substitute fakes.
type CmdRunner interface {
	Run(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error)
}

// ShRunner executes commands via `sh -c` when sh is on PATH, falling back
// to PowerShell (`pwsh`/`powershell -Command`) or `cmd /C` on machines
// without a POSIX shell (Windows without Git-bash on PATH). err is nil
// for any completed run (including non-zero exits); it is non-nil only
// when no verdict could be produced (spawn failure, kill, timeout).
type ShRunner struct {
	Dir      string
	EnvUnset []string
}

// dockerProbe reports whether a docker daemon is reachable. Tests stub it;
// production runs `docker info` with a 5s deadline.
var dockerProbe = func(ctx context.Context) bool {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "docker", "info")
	executil.HideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func (s ShRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	stdout, stderr, code, _, _, err := s.runWithStats(ctx, command, s.Dir, "", "")
	return stdout, stderr, code, err
}

func (s ShRunner) runWithStats(ctx context.Context, command, root, dockerImage, isolate string) (stdout, stderr string, code int, cpuMs int64, isolation string, err error) {
	if isolate == "docker" && dockerImage != "" {
		if !dockerProbe(ctx) {
			reason := "docker daemon unavailable"
			stdout, stderr, code, cpuMs, err := runShell(ctx, shellName(command), shellArgs(command), s.Dir, s.EnvUnset)
			note := "isolation: docker requested but unavailable (" + reason + "); ran host-contained"
			if stderr == "" {
				stderr = note
			} else {
				stderr += "\n" + note
			}
			return stdout, stderr, code, cpuMs, IsolationHostContained, err
		}
		vol := root
		if vol == "" {
			vol = s.Dir
		}
		if vol == "" {
			if wd, werr := os.Getwd(); werr == nil {
				vol = wd
			} else {
				vol = "."
			}
		}
		if abs, aerr := filepath.Abs(vol); aerr == nil {
			vol = abs
		}
		args := []string{"run", "--rm", "-v", vol + ":/work", "-w", "/work", dockerImage, "sh", "-c", command}
		stdout, stderr, code, cpuMs, err := runShell(ctx, "docker", args, "", nil)
		return stdout, stderr, code, cpuMs, IsolationDocker, err
	}
	if isolate == "docker" && dockerImage == "" {
		reason := "no docker image configured"
		stdout, stderr, code, cpuMs, err := runShell(ctx, shellName(command), shellArgs(command), s.Dir, s.EnvUnset)
		note := "isolation: docker requested but unavailable (" + reason + "); ran host-contained"
		if stderr == "" {
			stderr = note
		} else {
			stderr += "\n" + note
		}
		return stdout, stderr, code, cpuMs, IsolationHostContained, err
	}
	stdout, stderr, code, cpuMs, err = runShell(ctx, shellName(command), shellArgs(command), s.Dir, s.EnvUnset)
	return stdout, stderr, code, cpuMs, IsolationHostContained, err
}

func shellName(command string) string {
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh"
	}
	if _, err := exec.LookPath("pwsh"); err == nil {
		return "pwsh"
	}
	if _, err := exec.LookPath("powershell"); err == nil {
		return "powershell"
	}
	return "cmd"
}

func shellArgs(command string) []string {
	name := shellName(command)
	switch name {
	case "sh":
		return []string{"-c", command}
	case "pwsh", "powershell":
		return []string{"-NoProfile", "-NonInteractive", "-Command", command}
	default:
		return []string{"/C", command}
	}
}

// runShellFallback runs one command through a non-POSIX shell and maps the
// shell's own command-not-found report to the POSIX 127 convention, so a
// missing binary reads the same on every platform (sh reports 127; cmd
// reports 1/9009 and PowerShell reports 1 for the same condition).
func runShellFallback(ctx context.Context, name string, args []string, dir string, envUnset []string) (string, string, int, int64, error) {
	stdout, stderr, code, cpu, err := runShell(ctx, name, args, dir, envUnset)
	if err == nil && code != 0 && isShellNotFound(stdout, stderr) {
		return stdout, stderr, 127, cpu, nil
	}
	return stdout, stderr, code, cpu, err
}

// isShellNotFound reports whether the captured output is the shell itself
// refusing an unknown command (as opposed to the command running and
// failing). Markers cover cmd and PowerShell in English and Spanish; the
// PowerShell FullyQualifiedErrorId marker is unambiguous by construction.
func isShellNotFound(stdout, stderr string) bool {
	combined := stdout + "\n" + stderr
	for _, marker := range []string{
		"CommandNotFoundException",
		"not recognized as the name of a cmdlet",
		"no se reconoce como nombre de un cmdlet",
		"is not recognized as an internal or external command",
		"no se reconoce como un comando interno o externo",
	} {
		if strings.Contains(combined, marker) {
			return true
		}
	}
	return false
}

func cpuMillis(cmd *exec.Cmd) int64 {
	if cmd.ProcessState == nil {
		return 0
	}
	u := cmd.ProcessState.UserTime()
	s := cmd.ProcessState.SystemTime()
	return (u + s).Milliseconds()
}

func runShell(ctx context.Context, name string, args []string, dir string, envUnset []string) (string, string, int, int64, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(envUnset) > 0 {
		unset := map[string]bool{}
		for _, n := range envUnset {
			if n != "" {
				unset[n] = true
			}
		}
		if len(unset) > 0 {
			filtered := os.Environ()[:0:0]
			// rebuild without the unset names (exact match)
			filtered = filtered[:0]
			for _, kv := range os.Environ() {
				if i := strings.IndexByte(kv, '='); i > 0 {
					if unset[kv[:i]] {
						continue
					}
				}
				filtered = append(filtered, kv)
			}
			cmd.Env = filtered
		}
	}
	executil.HideWindow(cmd)
	return runShellCap(cmd, ctx, name, MaxOutputBytes)
}

// runShellCap starts cmd with per-stream output caps and waits. Extracted
// so tests can force the cap path with a tiny budget.
func runShellCap(cmd *exec.Cmd, ctx context.Context, name string, capBytes int) (string, string, int, int64, error) {
	outW := &capWriter{limit: capBytes}
	errbW := &capWriter{limit: capBytes}
	kill := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	outW.kill, errbW.kill = kill, kill
	cmd.Stdout, cmd.Stderr = outW, errbW
	if err := cmd.Start(); err != nil {
		return "", "", -1, 0, err
	}
	werr := cmd.Wait()
	cpu := cpuMillis(cmd)
	out, errb := outW.buf.String(), errbW.buf.String()
	if outW.overflow || errbW.overflow {
		return out, errb, -1, cpu, &outputCapError{bytes: outW.total + errbW.total}
	}
	if werr == nil {
		return out, errb, 0, cpu, nil
	}
	// A deadline kill surfaces as an ExitError on Windows (exit code 1)
	// and as a signal ExitError on Unix; the context check must come first
	// or a timeout masquerades as an ordinary failure.
	if ctx.Err() != nil {
		return out, errb, -1, cpu, fmt.Errorf("timed out: %w", ctx.Err())
	}
	var ee *exec.ExitError
	if errors.As(werr, &ee) {
		code := ee.ExitCode()
		if (name == "cmd" || name == "powershell" || name == "pwsh") && isShellNotFound(out, errb) {
			return out, errb, 127, cpu, nil
		}
		return out, errb, code, cpu, nil
	}
	return out, errb, -1, cpu, werr
}

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
		if g.Metric != nil {
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
		}
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
		if strings.TrimSpace(g.ID) != "" {
			for _, r := range g.ID {
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
					return fmt.Errorf("gates[%d]: id must match [A-Za-z0-9_-]+", i)
				}
			}
		}
	}
	if cfg.Execution != nil {
		for _, a := range cfg.Execution.Allowlist {
			if strings.TrimSpace(a) == "" {
				return fmt.Errorf("execution allowlist entries must not be empty")
			}
		}
	}
	return nil
}

// Run executes the gauntlet for one task, writing one append-only receipt
// per executed gate into evidenceDir. It stops at the first failure.
// Signature is UNCHANGED (probe calls it directly): policy is evaluated for
// every runner including fakes, and receipts carry the centralized verdict.
// runStamp links one Run to its candidate manifest. Empty means the
// probe path: receipts verify on hashes alone.
type runStamp struct {
	runID        string
	manifestFile string
}

func Run(ctx context.Context, cfg Config, evidenceDir, task, gitHead string, r CmdRunner) (Result, error) {
	return run(ctx, cfg, evidenceDir, task, gitHead, r, runStamp{}, "")
}

// SelectGates resolves which gates run: "" and "full" select everything
// (today's behavior), "quick" selects required gates only, and any other
// name must be declared by at least one gate's profiles list — an unknown
// profile is an error, never a silent subset.
func SelectGates(cfg Config, profile string) ([]Gate, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	switch profile {
	case "", ProfileFull:
		return append([]Gate(nil), cfg.Gates...), nil
	case ProfileQuick:
		var out []Gate
		for _, g := range cfg.Gates {
			if g.IsRequired() {
				out = append(out, g)
			}
		}
		return out, nil
	}
	declared := map[string]bool{}
	for _, g := range cfg.Gates {
		for _, p := range g.Profiles {
			declared[p] = true
		}
	}
	if !declared[profile] {
		var names []string
		for p := range declared {
			names = append(names, p)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("unknown profile %q (declared: %s; reserved: quick, full)", profile, strings.Join(names, ", "))
	}
	var out []Gate
	for _, g := range cfg.Gates {
		if g.IsRequired() {
			out = append(out, g)
			continue
		}
		for _, p := range g.Profiles {
			if p == profile {
				out = append(out, g)
				break
			}
		}
	}
	return out, nil
}

// changedFiles lists worktree paths from git status (tracked modifications
// plus untracked files, honoring git's own excludes). ok=false when git is
// unavailable: conditions are then unevaluable.
func changedFiles(ctx context.Context, r CmdRunner) ([]string, bool) {
	gctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, _, code, err := r.Run(gctx, "git status --porcelain=v1 -z --untracked-files=normal -- .")
	if err != nil || code != 0 {
		return nil, false
	}
	var paths []string
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		e := parts[i]
		if len(e) < 4 {
			continue
		}
		xy, path := e[:2], e[3:]
		if path == "" || xy == "!!" {
			continue
		}
		paths = append(paths, path)
		if xy[0] == 'R' {
			i++
		}
	}
	return paths, true
}

func matchesAnyGate(g Gate, changed []string) bool {
	for _, c := range changed {
		if matchesAny(c, g.When) {
			return true
		}
	}
	return false
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if ok, err := filepath.Match(p, path); err == nil && ok {
			return true
		}
		if ok, err := filepath.Match(p, filepath.Base(path)); err == nil && ok {
			return true
		}
	}
	return false
}

// runnerFacts maps a runner failure to verdict facts: a run killed for
// exceeding the output cap exhausted its resources, a deadline kill is an
// infrastructure timeout, and anything else never really ran.
func runnerFacts(gctx context.Context, rerr error) verdict.Facts {
	var capErr *outputCapError
	if errors.As(rerr, &capErr) {
		return verdict.Facts{Executed: true, ResourceExhausted: true}
	}
	if gctx.Err() != nil {
		return verdict.Facts{Executed: false, TimedOut: true}
	}
	return verdict.Facts{Executed: false, SpawnError: true}
}

func run(ctx context.Context, cfg Config, evidenceDir, task, gitHead string, r CmdRunner, stamp runStamp, profile string) (Result, error) {
	selected, err := SelectGates(cfg, profile)
	if err != nil {
		return Result{}, err
	}
	ordered := append([]Gate(nil), selected...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Layer < ordered[j].Layer })
	max := maxTimeout(cfg)
	dockerImage := ""
	var execEnvUnset []string
	if cfg.Execution != nil {
		dockerImage = cfg.Execution.DockerImage
		execEnvUnset = cfg.Execution.EnvUnset
	}

	// Execution conditions are evaluated once per run against the worktree.
	// Required gates always run: when-conditions only ever skip optional
	// gates, and unevaluable conditions fail open with a receipt note.
	var changed []string
	changedOK := false
	needsChanged := false
	for _, g := range ordered {
		if len(g.When) > 0 {
			needsChanged = true
			break
		}
	}
	if needsChanged {
		changed, changedOK = changedFiles(ctx, r)
	}

	var res Result
	for _, g := range ordered {
		if !g.IsRequired() && len(g.When) > 0 && changedOK && !matchesAnyGate(g, changed) {
			reason := fmt.Sprintf("no changed files match when %s", strings.Join(g.When, ", "))
			res.Outcomes = append(res.Outcomes, Outcome{Name: g.Name, Layer: g.Layer, Verdict: "skip", Status: "INCONCLUSIVE", Reason: "insufficient_evidence", Skipped: true, SkipReason: reason})
			res.Skipped = append(res.Skipped, g.Name)
			continue
		}
		condNote := ""
		if len(g.When) > 0 {
			if !changedOK {
				condNote = "when: execution conditions unevaluable (git unavailable), ran unconditionally"
			} else if g.IsRequired() && !matchesAnyGate(g, changed) {
				condNote = fmt.Sprintf("when: no changed files match %s, ran anyway (required gate is mandatory)", strings.Join(g.When, ", "))
			}
		}
		timeout := effectiveTimeout(g.TimeoutSec, max)
		classification := Classify(g.Command)
		refused := false
		refuseNote := ""
		if classification == ClassificationDestructive && !(cfg.Execution != nil && cfg.Execution.AllowDestructive) {
			refused = true
			refuseNote = "policy: destructive command refused"
		}
		if !refused && cfg.Execution != nil && len(cfg.Execution.Allowlist) > 0 {
			ft := firstToken(g.Command)
			allowed := false
			for _, a := range cfg.Execution.Allowlist {
				if ft == a {
					allowed = true
					break
				}
			}
			if !allowed {
				refused = true
				refuseNote = "policy: command not in allowlist"
			}
		}

		var stdout, stderr string
		var code, exit int
		var rerr error
		var cpuMs int64
		isolation := IsolationUnknown
		var start time.Time
		var dur time.Duration
		var facts verdict.Facts

		if refused {
			facts = verdict.Facts{Refused: true}
			code = -1
			exit = -1
			stderr = refuseNote
			start = time.Now().UTC()
			dur = 0
			if isShRunner(r) {
				isolation = IsolationHostContained
			}
		} else {
			if sh, ok := asShRunner(r); ok {
				mergedEnv := append(append([]string(nil), sh.EnvUnset...), execEnvUnset...)
				shGate := ShRunner{Dir: sh.Dir, EnvUnset: mergedEnv}
				gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
				start = time.Now().UTC()
				so, se, c, cpu, iso, err := shGate.runWithStats(gctx, g.Command, sh.Dir, dockerImage, g.Isolate)
				dur = time.Since(start)
				cancel()
				stdout, stderr, code, cpuMs, isolation, rerr = so, se, c, cpu, iso, err
				exit = code
				if rerr != nil {
					if exit == 0 {
						exit = -1
					}
					note := "gate could not run: " + rerr.Error()
					if stderr == "" {
						stderr = note
					} else {
						stderr += "\n" + note
					}
					facts = runnerFacts(gctx, rerr)
				} else if code != 0 {
					facts = verdict.Facts{Executed: true, ExitCode: code}
				} else {
					facts = verdict.Facts{Executed: true, ExitCode: 0}
				}
			} else {
				gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
				start = time.Now().UTC()
				so, se, c, err := r.Run(gctx, g.Command)
				dur = time.Since(start)
				cancel()
				stdout, stderr, code, rerr = so, se, c, err
				cpuMs = 0
				isolation = IsolationUnknown
				exit = code
				if rerr != nil {
					if exit == 0 {
						exit = -1
					}
					note := "gate could not run: " + rerr.Error()
					if stderr == "" {
						stderr = note
					} else {
						stderr += "\n" + note
					}
					facts = runnerFacts(gctx, rerr)
				} else if code != 0 {
					facts = verdict.Facts{Executed: true, ExitCode: code}
				} else {
					facts = verdict.Facts{Executed: true, ExitCode: 0}
				}
			}
		}

		// Metric gates (F2): the command must exit 0 AND the extracted
		// metric must satisfy the operator against the threshold.
		var metricValue, metricThreshold *float64
		var metricOp string
		metricFailed := false
		if !refused && rerr == nil && code == 0 && g.Metric != nil {
			metricOp = g.Metric.Operator
			value, err := extractMetric(g.Metric.Extract, stdout)
			if err != nil {
				metricFailed = true
				note := "metric: " + err.Error()
				if stderr == "" {
					stderr = note
				} else {
					stderr += "\n" + note
				}
			} else {
				metricValue = &value
				th := g.Metric.Threshold
				metricThreshold = &th
				ok := (g.Metric.Operator == OpAtLeast && value >= th) ||
					(g.Metric.Operator == OpAtMost && value <= th)
				if !ok {
					metricFailed = true
					note := fmt.Sprintf("metric %.2f does not meet %s %.2f", value, g.Metric.Operator, th)
					if stderr == "" {
						stderr = note
					} else {
						stderr += "\n" + note
					}
				}
			}
			if metricFailed {
				facts = verdict.Facts{Executed: true, ExitCode: 0, MetricViolated: true}
			}
		}
		// A non-zero exit without rerr was captured above; a metric gate that
		// failed its command keeps the exit-code facts (already deterministic).
		if !refused && g.Metric != nil && rerr == nil && code != 0 {
			// facts already Executed+ExitCode from above
			metricValue = nil
			metricThreshold = nil
			metricOp = ""
		}
		// Refused and spawn/timeout facts were set above; exit!=0 facts set;
		// otherwise facts is Executed+ExitCode 0 (pass unless metric failed).

		v := verdict.Adjudicate(facts)
		legacy := v.Legacy()

		rec := receipts.Receipt{
			Task:            task,
			Gate:            g.Name,
			Layer:           g.Layer,
			Command:         g.Command,
			ExitCode:        exit,
			Verdict:         legacy,
			StartedAt:       start,
			DurationMS:      dur.Milliseconds(),
			GitHead:         gitHead,
			MetricValue:     metricValue,
			MetricThreshold: metricThreshold,
			MetricOperator:  metricOp,
			Status:          string(v.Status),
			Reason:          string(v.Reason),
			RunID:           stamp.runID,
			Manifest:        stamp.manifestFile,
			CPUTimeMS:       cpuMs,
			Isolation:       isolation,
			Classification:  classification,
			Schema:          receipts.ReceiptSchemaVersion,
		}
		if condNote != "" {
			if stderr == "" {
				stderr = condNote
			} else {
				stderr += "\n" + condNote
			}
		}
		rec.Stdout, rec.StdoutSHA = receipts.Capture(stdout)
		rec.Stderr, rec.StderrSHA = receipts.Capture(stderr)

		path, err := receipts.Write(evidenceDir, rec)
		if err != nil {
			return res, fmt.Errorf("gate %s: writing receipt: %w", g.Name, err)
		}
		// ReceiptID is assigned inside Write; re-read to link outcomes.
		recID := ""
		if rr, rerr := receipts.ReadFile(path); rerr == nil {
			recID = rr.ID
		}
		res.Executed++
		res.Receipts = append(res.Receipts, path)
		res.Outcomes = append(res.Outcomes, Outcome{Name: g.Name, Layer: g.Layer, Verdict: legacy, DurationMS: dur.Milliseconds(), Status: string(v.Status), Reason: string(v.Reason), ReceiptPath: path, ReceiptID: recID, TimeoutSec: timeout, Advisory: g.EffectiveOnFail() == OnFailAdvisory})
		if legacy == receipts.VerdictFail {
			if g.EffectiveOnFail() == OnFailAdvisory {
				// Advisory failure: recorded honestly, run continues.
				continue
			}
			res.Failed = true
			res.FailedGate = g.Name
			res.Status = string(v.Status)
			res.Reason = string(v.Reason)
			return res, nil
		}
		res.Passed++
	}
	res.Status = string(verdict.StatusPass)
	res.Reason = string(verdict.ReasonOK)
	return res, nil
}

func isShRunner(r CmdRunner) bool {
	_, ok1 := r.(ShRunner)
	if ok1 {
		return true
	}
	_, ok2 := r.(*ShRunner)
	return ok2
}

func asShRunner(r CmdRunner) (ShRunner, bool) {
	if v, ok := r.(ShRunner); ok {
		return v, true
	}
	if v, ok := r.(*ShRunner); ok && v != nil {
		return *v, true
	}
	return ShRunner{}, false
}

// GitHead returns the current HEAD hash, or "" outside a repository.
func GitHead(ctx context.Context, r CmdRunner) string {
	out, _, _, err := r.Run(ctx, "git rev-parse --verify HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

var metricNumberRe = regexp.MustCompile(`[-+]?[0-9]*\.?[0-9]+`)

// extractMetric pulls one number out of a metric gate's stdout. Both modes
// read the last non-empty line; the percent mode strips a trailing "%"
// first. Within that line the LAST numeric token wins.
func extractMetric(extract, stdout string) (float64, error) {
	lines := strings.Split(stdout, "\n")
	var last string
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			last = s
			break
		}
	}
	if last == "" {
		return 0, errors.New("could not extract a metric: the command printed nothing")
	}
	if extract == ExtractLastLinePercent {
		last = strings.TrimSuffix(last, "%")
	}
	matches := metricNumberRe.FindAllString(last, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("could not extract a metric from %q", last)
	}
	v, err := strconv.ParseFloat(matches[len(matches)-1], 64)
	if err != nil {
		return 0, fmt.Errorf("could not extract a metric from %q", last)
	}
	return v, nil
}
