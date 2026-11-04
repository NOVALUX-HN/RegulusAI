package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/receipts"
)

// MaxReviewCorrections bounds one review lineage (chart pro-harness-rdd):
// two drifted approvals consume the budget; past it only a human
// decides. Unbounded retry is how reviews become rubber stamps.
const MaxReviewCorrections = 2

// reviewLineage is one frozen review transaction: the candidate bytes
// (per-file sha256) plus the approval state. Approvals bind to these
// exact bytes — any drift refutes reuse.
type reviewLineage struct {
	ID          string            `json:"id"`
	Task        string            `json:"task"`
	Head        string            `json:"head"`
	Files       map[string]string `json:"files"`
	Corrections int               `json:"corrections"`
	Approved    bool              `json:"approved"`
	Verdict     string            `json:"verdict,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// reviewFrozenDirs are never candidate bytes: version control, local
// runtime state, build artifacts, vendored dependencies. All volatile,
// none reviewed.
var reviewFrozenSkip = map[string]bool{
	".git": true, ".regulus": true, "binaries": true, "node_modules": true,
}

// reviewLineageFile resolves the lineage record, jailing the task id
// inside the review store (same traversal discipline as evidence).
func reviewLineageFile(root, task, id string) (string, error) {
	slug := receipts.Slug(task)
	if slug == "" || strings.Contains(slug, "..") {
		return "", fmt.Errorf("review: invalid task id %q", task)
	}
	if strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") || strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("review: invalid lineage id %q", id)
	}
	base, err := filepath.Abs(filepath.Join(root, ".regulus", "review", slug))
	if err != nil {
		return "", err
	}
	p := filepath.Join(root, ".regulus", "review", slug, id+".json")
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return "", fmt.Errorf("review: lineage escapes review dir: %q", id)
	}
	return p, nil
}

// reviewFreezeBytes hashes the candidate: every regular file under
// root except volatile dirs, keyed by slash-relative path. No git
// dependency — hermetic in temp dirs and uniform on real repos. The
// HEAD (when git answers) rides along as provenance display only;
// drift is decided by bytes, never by refs.
func reviewFreezeBytes(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if reviewFrozenSkip[filepath.Base(p)] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func reviewLoadLineage(root, task, id string) (reviewLineage, error) {
	var l reviewLineage
	p, err := reviewLineageFile(root, task, id)
	if err != nil {
		return l, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return l, fmt.Errorf("review: no such lineage %q for task %q", id, task)
	}
	if err := json.Unmarshal(raw, &l); err != nil {
		return l, fmt.Errorf("review: lineage %q is corrupt: %v", id, err)
	}
	return l, nil
}

func reviewSaveLineage(root string, l reviewLineage) error {
	p, err := reviewLineageFile(root, l.Task, l.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "lineage-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, p)
}

// reviewDrift compares live bytes against the frozen record, naming
// the first drift class found (added/removed/changed, sorted): one
// honest reason, not a dump.
func reviewDrift(frozen, live map[string]string) string {
	for _, f := range sortedKeys(live) {
		if _, ok := frozen[f]; !ok {
			return "added " + f
		}
	}
	for _, f := range sortedKeys(frozen) {
		h, ok := live[f]
		if !ok {
			return "removed " + f
		}
		if h != frozen[f] {
			return "changed " + f
		}
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// reviewMintID mints lineage ids (rev_ + hex nanos, the les_/exp_
// family convention): unique without coordination, sortable by birth.
func reviewMintID(now time.Time) string {
	return "rev_" + strconv.FormatInt(now.UnixNano(), 16)
}

// reviewStartCmd implements `review start --task <t>`: freeze the
// candidate bytes under a new lineage and print only its id (the
// handle every later subcommand takes).
func (c CLI) reviewStartCmd(args []string) int {
	fs := flag.NewFlagSet("review start", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "task the lineage belongs to (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*task) == "" || !validTaskID(*task) || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus review start --task <t>")
		return 2
	}
	files, err := reviewFreezeBytes(c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "review start: %v\n", err)
		return 1
	}
	head := "nogit"
	if h, _, _, gitOK := reviewFreeze(c); gitOK && strings.TrimSpace(h) != "" {
		head = strings.TrimSpace(h)
	}
	l := reviewLineage{
		ID: reviewMintID(time.Now().UTC()), Task: *task, Head: head,
		Files: files, CreatedAt: time.Now().UTC(),
	}
	if err := reviewSaveLineage(c.dir(), l); err != nil {
		fmt.Fprintf(c.Stderr, "review start: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, l.ID)
	return 0
}

// reviewApproveCmd implements `review approve --task <t> --lineage <l>
// --verdict pass|fail`: approval binds to the frozen bytes. Drift
// refutes reuse and consumes one correction round (N/2 named); past
// the budget only a human decides. Exit 0 approved, 1 refused.
func (c CLI) reviewApproveCmd(args []string) int {
	fs := flag.NewFlagSet("review approve", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "task the lineage belongs to (required)")
	lineage := fs.String("lineage", "", "lineage id from review start (required)")
	verdict := fs.String("verdict", "", "pass | fail (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*task) == "" || !validTaskID(*task) ||
		strings.TrimSpace(*lineage) == "" || (*verdict != "pass" && *verdict != "fail") || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus review approve --task <t> --lineage <l> --verdict pass|fail")
		return 2
	}
	l, err := reviewLoadLineage(c.dir(), *task, *lineage)
	if err != nil {
		fmt.Fprintf(c.Stderr, "review approve: %v\n", err)
		return 1
	}
	live, err := reviewFreezeBytes(c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "review approve: %v\n", err)
		return 1
	}
	if d := reviewDrift(l.Files, live); d != "" {
		l.Corrections++
		if l.Corrections > MaxReviewCorrections {
			if err := reviewSaveLineage(c.dir(), l); err != nil {
				fmt.Fprintf(c.Stderr, "review approve: %v\n", err)
			}
			fmt.Fprintf(c.Stderr, "review approve: correction budget exhausted for lineage %s (drift: %s) — a human decision is required\n", l.ID, d)
			return 1
		}
		if err := reviewSaveLineage(c.dir(), l); err != nil {
			fmt.Fprintf(c.Stderr, "review approve: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stderr, "review approve: candidate drifted since freeze (%s) — approval refused, correction %d/%d consumed\n", d, l.Corrections, MaxReviewCorrections)
		return 1
	}
	l.Approved = true
	l.Verdict = *verdict
	if err := reviewSaveLineage(c.dir(), l); err != nil {
		fmt.Fprintf(c.Stderr, "review approve: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "review: lineage %s approved (%s) — frozen bytes only, any later change invalidates this approval\n", l.ID, *verdict)
	return 0
}

// reviewRefuteCmd implements `review refute --task <t> --lineage <l>
// --file <p>`: the adversarial sort. A finding touching the frozen
// set is candidate-caused (a blocker unless disproven); anything
// outside it is pre-existing — a follow-up, never a blocker. Exit 0
// with the sort, never a verdict.
func (c CLI) reviewRefuteCmd(args []string) int {
	fs := flag.NewFlagSet("review refute", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "task the lineage belongs to (required)")
	lineage := fs.String("lineage", "", "lineage id from review start (required)")
	file := fs.String("file", "", "finding path to sort (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*task) == "" || !validTaskID(*task) ||
		strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*file) == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus review refute --task <t> --lineage <l> --file <p>")
		return 2
	}
	l, err := reviewLoadLineage(c.dir(), *task, *lineage)
	if err != nil {
		fmt.Fprintf(c.Stderr, "review refute: %v\n", err)
		return 1
	}
	p := filepath.ToSlash(filepath.Clean(strings.TrimSpace(*file)))
	if _, ok := l.Files[p]; ok {
		fmt.Fprintf(c.Stdout, "review: candidate-caused: %s is in the frozen set — treat as blocker unless disproven\n", p)
		return 0
	}
	fmt.Fprintf(c.Stdout, "review: pre-existing: %s is outside the frozen set — follow-up, never a blocker\n", p)
	return 0
}
