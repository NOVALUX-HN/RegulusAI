package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/crap"
)

// auditUntestedComplexityFloor bounds the should-fix tier: untested
// logic below it is trivial enough to ride as nice-to-have. Five
// means several decision points — the "branch, loop, parser" band the
// worker's Necessity Ladder calls non-trivial — not every uncovered
// getter.
const auditUntestedComplexityFloor = 5

// entryPoints maps the repo first (ponytail-audit order: map, then
// rank): every cmd/<name> directory holding Go sources is an entry
// point. Sorted, never empty-shaped: no cmd tree yields nil so the
// caller prints the honest "(none found)".
func entryPoints(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		srcs, err := filepath.Glob(filepath.Join(root, "cmd", e.Name(), "*.go"))
		if err != nil || len(srcs) == 0 {
			continue
		}
		out = append(out, "cmd/"+e.Name())
	}
	sort.Strings(out)
	return out
}

// auditTier ranks one scored function: over the gate max is must-fix
// (consistent with the gauntlet — the same code fails layer 7),
// untested non-trivial logic is should-fix, everything else in the
// top rides as nice-to-have.
func auditTier(f crap.Scored, max float64) string {
	if f.CRAP > max {
		return "must-fix"
	}
	if f.Coverage == 0 && f.Complexity >= auditUntestedComplexityFloor {
		return "should-fix"
	}
	return "nice"
}

// auditTierFor ranks by criticidad: core routes fail at max-critical,
// the rest at max-standard. The global max stays as the ceiling: over
// it is must-fix anywhere (raise --max to let the per-tier thresholds
// distinguish).
func auditTierFor(f crap.Scored, maxCrit, maxStd, globalMax float64) string {
	per := maxStd
	if crap.IsCriticalFile(f.File) {
		per = maxCrit
	}
	if f.CRAP > per || f.CRAP > globalMax {
		return "must-fix"
	}
	if (!f.Measured || f.Coverage == 0) && f.Complexity >= auditUntestedComplexityFloor {
		return "should-fix"
	}
	return "nice"
}

// coverageLabel renders coverage for humans: unmeasured functions are
// "unknown", never 0.
func coverageLabel(f crap.Scored) string {
	if !f.Measured {
		return "unknown"
	}
	return fmt.Sprintf("%.1f%%", f.Coverage*100)
}

// coveragePtr renders coverage for the hotspot JSON: nil (null) when
// unmeasured, never 0.
func coveragePtr(f crap.Scored) *float64 {
	if !f.Measured {
		return nil
	}
	v := f.Coverage
	return &v
}

// readProfileBytes reads a cover profile the same way Analyze does:
// relative to the caller cwd first, then to the project root.
func readProfileBytes(dir, profile string) ([]byte, error) {
	if data, err := os.ReadFile(profile); err == nil {
		return data, nil
	}
	return os.ReadFile(filepath.Join(dir, profile))
}

// sourceInfo returns the profile fuente (full + short sha256) and fecha
// (RFC3339 UTC now). A missing profile yields empty hashes but still a
// timestamp; callers print "unknown" for the empty hash.
func sourceInfo(dir, profile string) (full, short, when string) {
	when = time.Now().UTC().Format(time.RFC3339)
	data, err := readProfileBytes(dir, profile)
	if err != nil {
		return "", "", when
	}
	sum := sha256.Sum256(data)
	full = hex.EncodeToString(sum[:])
	return full, full[:8], when
}

// loadWaivers resolves the --exceptions file like the profile: cwd
// first, then the project root. Blank or missing means no waivers.
func loadWaivers(dir, path string) map[string]bool {
	if strings.TrimSpace(path) == "" {
		return map[string]bool{}
	}
	if _, err := os.Stat(path); err == nil {
		return crap.LoadWaivers(path)
	}
	return crap.LoadWaivers(filepath.Join(dir, path))
}

// hotspotFunc is one ranked row of the hotspot report.
type hotspotFunc struct {
	File     string   `json:"file"`
	Func     string   `json:"func"`
	CX       int      `json:"cx"`
	Coverage *float64 `json:"coverage"`
	CRAP     float64  `json:"crap"`
	Tier     string   `json:"tier"`
}

// hotspotReport is the --hotspot-report artifact: fuente, fecha and the
// ranked top reusing the same analysis as stdout.
type hotspotReport struct {
	GeneratedAt   string        `json:"generated_at"`
	Profile       string        `json:"profile"`
	ProfileSHA256 string        `json:"profile_sha256"`
	Top           int           `json:"top"`
	Functions     []hotspotFunc `json:"functions"`
}

// writeHotspotReport writes the JSON artifact, creating parents.
func writeHotspotReport(path, profile, shaFull string, top int, funcs []hotspotFunc) error {
	rep := hotspotReport{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Profile:       profile,
		ProfileSHA256: shaFull,
		Top:           top,
		Functions:     funcs,
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// auditCmd implements `regulus audit --profile <coverprofile>
// [--top N] [--max M] [--max-critical C] [--max-standard S]
// [--hotspot-report P] [--exceptions E]`: the repo-wide ranked audit.
// Map first (entry points), then rank (CRAP tiers), then name what to
// fix first. Informational by design — exit 0 with a verdict, never a
// gate: the gauntlet owns failing, the audit owns prioritizing.
func (c CLI) auditCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus audit --profile <coverprofile> [--max N] [--top N]")
		return 0
	}
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	profile := fs.String("profile", "", "go cover profile path (required)")
	max := fs.Float64("max", 30, "global ceiling: CRAP above this is must-fix anywhere (raise it to let per-tier thresholds distinguish)")
	maxCrit := fs.Float64("max-critical", 30, "CRAP above this is must-fix on core routes (cmd/*, internal/decisions|gates|receipts|evidence/*)")
	maxStd := fs.Float64("max-standard", 50, "CRAP above this is must-fix off the core routes")
	top := fs.Int("top", 10, "how many worst functions to rank")
	hotspot := fs.String("hotspot-report", "", "write JSON hotspot report {generated_at, profile, profile_sha256, top, functions} to this path")
	exceptions := fs.String("exceptions", "docs/quality-exceptions.md", "waiver file (file:func motivo expira aprueba); waived functions discount the Max")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profile == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus audit --profile <coverprofile> [--top N] [--max M] [--max-critical C] [--max-standard S] [--hotspot-report P] [--exceptions E]")
		return 2
	}
	rep, err := crap.Analyze(c.dir(), *profile)
	if err != nil {
		fmt.Fprintf(c.Stderr, "audit: %v\n", err)
		return 1
	}
	waived := loadWaivers(c.dir(), *exceptions)
	full, short, when := sourceInfo(c.dir(), *profile)
	sha := short
	if sha == "" {
		sha = "unknown"
	}
	fmt.Fprintf(c.Stdout, "source: %s sha256:%s generated_at:%s\n", *profile, sha, when)
	fmt.Fprintln(c.Stdout, "Entry points:")
	for _, ep := range entryPoints(c.dir()) {
		fmt.Fprintf(c.Stdout, "  - %s\n", ep)
	}
	if len(entryPoints(c.dir())) == 0 {
		fmt.Fprintln(c.Stdout, "  (none found)")
	}
	fmt.Fprintln(c.Stdout, "Ranked findings:")
	var must, should, nice, nwaived int
	first := ""
	var hot []hotspotFunc
	for i, f := range rep.Functions {
		if i >= *top {
			break
		}
		tier := auditTierFor(f, *maxCrit, *maxStd, *max)
		if waived[crap.WaiverKey(f.File, f.Name)] {
			tier = "waived"
		}
		switch tier {
		case "must-fix":
			must++
		case "should-fix":
			should++
		case "waived":
			nwaived++
		default:
			nice++
		}
		if first == "" && tier != "waived" {
			first = f.Name
		}
		fmt.Fprintf(c.Stdout, "  - [%s] %s:%d %s (complexity %d, coverage %s) CRAP %.2f\n",
			tier, f.File, f.StartLine, f.Name, f.Complexity, coverageLabel(f), f.CRAP)
		hot = append(hot, hotspotFunc{File: f.File, Func: f.Name, CX: f.Complexity, Coverage: coveragePtr(f), CRAP: f.CRAP, Tier: tier})
	}
	if *hotspot != "" {
		if err := writeHotspotReport(*hotspot, *profile, full, *top, hot); err != nil {
			fmt.Fprintf(c.Stderr, "audit: %v\n", err)
			return 1
		}
	}
	if first == "" {
		if nwaived > 0 {
			fmt.Fprintf(c.Stdout, "audit: 0 must-fix | 0 should-fix | 0 nice (+%d waived) — clean\n", nwaived)
			return 0
		}
		fmt.Fprintln(c.Stdout, "audit: 0 must-fix | 0 should-fix | 0 nice — clean")
		return 0
	}
	if nwaived > 0 {
		fmt.Fprintf(c.Stdout, "audit: %d must-fix | %d should-fix | %d nice (+%d waived) — fix %s first\n",
			must, should, nice, nwaived, first)
		return 0
	}
	fmt.Fprintf(c.Stdout, "audit: %d must-fix | %d should-fix | %d nice — fix %s first\n",
		must, should, nice, first)
	return 0
}
