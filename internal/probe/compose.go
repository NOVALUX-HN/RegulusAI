package probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/reflect"
)

// Compose runs a base command and two variants through the SAME path
// `gate run` uses (gates.Run, so each of the three runs writes a real
// receipt under .regulus/evidence/<task>/), then deterministically
// merges the non-overlapping changes of A and B relative to base and
// flags the overlapping ones as conflicts. The merge runs per field
// (exit code, stdout lines, stderr lines): a field unchanged in both
// variants keeps the base value; a field changed in exactly one variant
// takes that variant's value; a field changed in both identically takes
// that value without conflict; a field changed in both differently is a
// conflict that keeps the base value and records both variants.
//
// It returns the human-readable composed report, whether any field
// conflicted, whether all three runs executed (ran), and an error.
// ran is true only when base, A and B all executed; conflicts is
// meaningful only when ran is true. Conflicts are evidence, not
// failure: when ran is true the error is nil even if conflicts is true.
// One budget entry (Kind=probe, Outcome=probe-run) with the sha256 of
// the composed report in Detail records the compose. Refusals (empty or
// invalid task, empty gate/base/commands, unknown gate, missing config,
// run errors) return an error with ran=false and record nothing —
// except the evolution-cap refusal, which records RefusedCap exactly
// like Contrast and runs nothing.
func Compose(root, task, gate, base, cmdA, cmdB string, now time.Time) (string, bool, bool, error) {
	return ComposeWithRunner(root, task, gate, base, cmdA, cmdB, now, runner())
}

// ComposeWithRunner is Compose with an injected command runner (the CLI
// passes its own runner so tests can substitute fakes).
func ComposeWithRunner(root, task, gate, base, cmdA, cmdB string, now time.Time, r gates.CmdRunner) (string, bool, bool, error) {
	fail := func(err error) (string, bool, bool, error) {
		return "", false, false, err
	}
	if strings.TrimSpace(task) == "" {
		return fail(fmt.Errorf("probe compose: task must not be empty"))
	}
	if !taskIDRe.MatchString(task) || strings.Contains(task, "..") {
		return fail(fmt.Errorf("probe compose: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')", task))
	}
	if strings.TrimSpace(gate) == "" {
		return fail(fmt.Errorf("probe compose: gate must not be empty"))
	}
	if strings.TrimSpace(base) == "" || strings.TrimSpace(cmdA) == "" || strings.TrimSpace(cmdB) == "" {
		return fail(fmt.Errorf("probe compose: --base, --a and --b commands must not be empty"))
	}
	if r == nil {
		r = gates.ShRunner{}
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Cap check (PRD R-E4), mirroring Contrast: composes consume the
	// evolution budget like reflect proposals; refusals are recorded
	// but free, and no command runs on refusal.
	cap := reflect.EvolutionCap(root)
	today, err := budget.TodayEvolutionCount(root, now)
	if err != nil {
		return fail(err)
	}
	if today >= cap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindProbe,
			Timestamp: now.UTC(),
			Task:      task,
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("cap %d reached", cap),
		}); err != nil {
			return fail(err)
		}
		return fail(fmt.Errorf("probe compose: evolution cap reached: %d run(s) today, cap %d (regulus.json evolution.max_per_day)", today, cap))
	}
	tcap := reflect.TaskCap(root)
	tcount, err := budget.TaskEvolutionCount(root, task, now)
	if err != nil {
		return fail(err)
	}
	if tcount >= tcap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindProbe,
			Timestamp: now.UTC(),
			Task:      task,
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("task cap %d reached", tcap),
		}); err != nil {
			return fail(err)
		}
		return fail(fmt.Errorf("probe compose: task evolution cap reached: task %q has %d run(s) today, task cap %d (regulus.json evolution.max_per_task)", task, tcount, tcap))
	}

	cfg, err := gates.Load(filepath.Join(root, "regulus.json"))
	if err != nil {
		return fail(fmt.Errorf("probe compose: %v", err))
	}
	var orig *gates.Gate
	for i := range cfg.Gates {
		if cfg.Gates[i].Name == gate {
			orig = &cfg.Gates[i]
			break
		}
	}
	if orig == nil {
		return fail(fmt.Errorf("probe compose: unknown gate %q", gate))
	}

	head := gates.GitHead(context.Background(), r)
	// Probe runs land in their own subdirectory: gate evidence stays
	// pure (reflect's evidence gate, gate-centric audits), and consumers
	// (evolve pack, human inspection) can tell behavioral contrasts
	// apart from gate verdicts at a glance.
	evidenceDir := filepath.Join(root, ".regulus", "evidence", receipts.Slug(task), "probe")
	run := func(cmd string) (receipts.Receipt, error) {
		return runProbeReceipt("probe compose", *orig, evidenceDir, task, head, cmd, r)
	}
	recBase, err := run(base)
	if err != nil {
		return fail(fmt.Errorf("probe compose: run base: %v", err))
	}
	recA, err := run(cmdA)
	if err != nil {
		return fail(fmt.Errorf("probe compose: run A: %v", err))
	}
	recB, err := run(cmdB)
	if err != nil {
		return fail(fmt.Errorf("probe compose: run B: %v", err))
	}

	report, conflicts := renderComposed(recBase, recA, recB, base, cmdA, cmdB, gate, task)
	sum := sha256.Sum256([]byte(report))
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindProbe,
		Timestamp: now.UTC(),
		Task:      task,
		Outcome:   budget.OutcomeProbeRun,
		Detail:    fmt.Sprintf("compose task %s gate %s sha256 %s conflicts=%t", task, gate, hex.EncodeToString(sum[:]), conflicts),
	}); err != nil {
		return fail(fmt.Errorf("probe compose: %v", err))
	}
	return report, conflicts, true, nil
}

// lineHunk is one conflicting stdout/stderr region: the base lines kept
// in the merge plus the differing A and B replacements.
type lineHunk struct {
	field       string // "stdout" or "stderr"
	b0, b1      int    // base line range (0-based, b1 exclusive)
	base, a, b  []string
	insertLabel bool // true when b0 == b1 (pure insertion point)
}

// renderComposed merges the three receipts field by field and renders
// the deterministic report: one ## Composed section per field in
// exit_code/stdout/stderr order, then ## Conflicts.
func renderComposed(recBase, recA, recB receipts.Receipt, base, cmdA, cmdB, gate, task string) (string, bool) {
	exitVal, exitConflict := mergeScalar(recBase.ExitCode, recA.ExitCode, recB.ExitCode)
	stdMerged, stdHunks := mergeLineField("stdout", recBase.Stdout, recA.Stdout, recB.Stdout)
	errMerged, errHunks := mergeLineField("stderr", recBase.Stderr, recA.Stderr, recB.Stderr)

	var sb strings.Builder
	fmt.Fprintf(&sb, "compose gate %q task %q\n", gate, task)
	fmt.Fprintf(&sb, "--- base: %s\n", base)
	fmt.Fprintf(&sb, "--- A: %s\n", cmdA)
	fmt.Fprintf(&sb, "--- B: %s\n", cmdB)
	sb.WriteString("## Composed exit_code\n")
	fmt.Fprintf(&sb, "%d\n", exitVal)
	sb.WriteString("## Composed stdout\n")
	sb.WriteString(renderLines(stdMerged))
	sb.WriteString("## Composed stderr\n")
	sb.WriteString(renderLines(errMerged))
	sb.WriteString("## Conflicts\n")
	conflicts := exitConflict || len(stdHunks) > 0 || len(errHunks) > 0
	if !conflicts {
		sb.WriteString("(no conflicts)\n")
		return sb.String(), false
	}
	if exitConflict {
		fmt.Fprintf(&sb, "- exit_code: base %d, A %d, B %d\n", recBase.ExitCode, recA.ExitCode, recB.ExitCode)
	}
	for _, h := range append(append([]lineHunk{}, stdHunks...), errHunks...) {
		sb.WriteString("- " + hunkTitle(h) + " (base kept):\n")
		sb.WriteString("  base:\n")
		sb.WriteString(indentLines(h.base))
		sb.WriteString("  A:\n")
		sb.WriteString(indentLines(h.a))
		sb.WriteString("  B:\n")
		sb.WriteString(indentLines(h.b))
	}
	return sb.String(), true
}

// mergeScalar merges one scalar field: unchanged in both takes base,
// changed in exactly one takes that variant, changed in both identically
// takes that value, changed in both differently conflicts on base.
func mergeScalar[T comparable](base, a, b T) (T, bool) {
	aChanged := a != base
	bChanged := b != base
	switch {
	case !aChanged && !bChanged:
		return base, false
	case aChanged && !bChanged:
		return a, false
	case !aChanged && bChanged:
		return b, false
	case a == b:
		return a, false
	default:
		return base, true
	}
}

// mergeLineField three-way merges one line-oriented field (stdout,
// stderr) relative to base. Non-overlapping changes from A and B both
// apply; identical changes on both sides apply once; overlapping
// different changes keep the base lines and are returned as hunks.
func mergeLineField(field, base, a, b string) ([]string, []lineHunk) {
	bl, al, vl := splitLines(base), splitLines(a), splitLines(b)
	if equalLines(al, bl) && equalLines(vl, bl) {
		return bl, nil
	}
	if equalLines(al, bl) {
		return vl, nil
	}
	if equalLines(vl, bl) {
		return al, nil
	}
	if equalLines(al, vl) {
		return al, nil
	}
	return mergeSegments(field, bl, al, vl)
}

// seg is one variant's edit of a base range: base[b0:b1] becomes lines.
type seg struct {
	b0, b1 int
	lines  []string
	fromA  bool
}

// mergeSegments builds both variants' edits of the base lines, sorts
// them by base range, clusters overlapping ones and walks the merge
// left to right.
func mergeSegments(field string, bl, al, vl []string) ([]string, []lineHunk) {
	segs := buildSegs(bl, al, true)
	segs = append(segs, buildSegs(bl, vl, false)...)
	sort.Slice(segs, func(i, j int) bool {
		if segs[i].b0 != segs[j].b0 {
			return segs[i].b0 < segs[j].b0
		}
		if segs[i].b1 != segs[j].b1 {
			return segs[i].b1 < segs[j].b1
		}
		return segs[i].fromA && !segs[j].fromA
	})
	return walkClusters(field, bl, segs)
}

// buildSegs turns one variant's opcodes into its non-equal seg edits.
func buildSegs(bl, variant []string, fromA bool) []seg {
	var segs []seg
	for _, op := range diffOpcodes(bl, variant) {
		if op.tag != 'e' {
			segs = append(segs, seg{op.b0, op.b1, append([]string{}, variant[op.v0:op.v1]...), fromA})
		}
	}
	return segs
}

// walkClusters clusters segments that touch the same base region —
// strict base overlap, or two insertions at the same point; an
// insertion that merely abuts a change on the other side stays
// separate so both apply — and flushes each cluster in order.
func walkClusters(field string, bl []string, segs []seg) ([]string, []lineHunk) {
	m := &merger{field: field, bl: bl}
	cluster := []seg{segs[0]}
	clusterEnd := segs[0].b1
	for _, s := range segs[1:] {
		if s.b0 < clusterEnd || (s.b0 == clusterEnd && s.b0 == s.b1 && hasInsertAt(cluster, s.b0)) {
			cluster = append(cluster, s)
			if s.b1 > clusterEnd {
				clusterEnd = s.b1
			}
			continue
		}
		m.flush(cluster, clusterEnd)
		cluster = []seg{s}
		clusterEnd = s.b1
	}
	m.flush(cluster, clusterEnd)
	m.merged = append(m.merged, bl[m.pos:]...)
	return m.merged, m.hunks
}

// hasInsertAt reports whether a cluster already inserts at base point p.
func hasInsertAt(cluster []seg, p int) bool {
	for _, s := range cluster {
		if s.b0 == p && s.b1 == p {
			return true
		}
	}
	return false
}

// merger carries the left-to-right merge state: position in the base
// lines, the merged output and the conflict hunks so far.
type merger struct {
	field  string
	bl     []string
	merged []string
	hunks  []lineHunk
	pos    int
}

// flush applies one cluster: a single edit (or several identical
// ones) lands in the output; conflicting edits keep the base lines
// and are recorded as a hunk for the caller to report.
func (m *merger) flush(cluster []seg, end int) {
	m.merged = append(m.merged, m.bl[m.pos:cluster[0].b0]...)
	m.pos = end
	if len(cluster) == 1 || identicalCluster(cluster) {
		m.merged = append(m.merged, cluster[0].lines...)
		return
	}
	m.merged = append(m.merged, m.bl[cluster[0].b0:end]...)
	m.recordConflict(cluster, end)
}

// identicalCluster reports whether every edit in the cluster is the
// same base range carrying the same lines.
func identicalCluster(cluster []seg) bool {
	for _, s := range cluster[1:] {
		if s.b0 != cluster[0].b0 || s.b1 != cluster[0].b1 || !equalLines(s.lines, cluster[0].lines) {
			return false
		}
	}
	return true
}

// recordConflict keeps the base lines and files the hunk with each
// side's variant lines.
func (m *merger) recordConflict(cluster []seg, end int) {
	var ha, hb []string
	for _, s := range cluster {
		if s.fromA {
			ha = append(ha, s.lines...)
		} else {
			hb = append(hb, s.lines...)
		}
	}
	m.hunks = append(m.hunks, lineHunk{
		field: m.field, b0: cluster[0].b0, b1: end,
		base: append([]string{}, m.bl[cluster[0].b0:end]...),
		a:    ha, b: hb, insertLabel: cluster[0].b0 == end,
	})
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// opcode is one LCS-aligned edit of a variant relative to base: 'e'
// keeps base[b0:b1], 'r' replaces them with lines, 'd' deletes them,
// 'i' inserts lines at b0 (b0 == b1).
type opcode struct {
	tag    byte
	b0, b1 int
	v0, v1 int
}

// diffOpcodes aligns variant lines to base lines with the same LCS
// approach diffLines uses, grouped into equal/replace/delete/insert
// runs that partition the base range.
// diffOpcodes aligns variant lines to base lines with the same LCS
// approach diffLines uses, grouped into equal/replace/delete/insert
// runs that partition the base range.
func diffOpcodes(base, v []string) []opcode {
	return groupOpcodes(lcsWalk(base, v, lcsTable(base, v)))
}

// lcsStep is one aligned line: keep, delete (from base) or insert.
type lcsStep struct {
	tag  byte // 'e' keep, 'd' delete base, 'i' insert variant
	b, v int
}

// lcsTable fills the longest-common-subsequence DP table of base and v.
func lcsTable(base, v []string) [][]int {
	n, m := len(base), len(v)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if base[i] == v[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	return lcs
}

// lcsWalk turns the DP table into per-line keep/delete/insert steps.
func lcsWalk(base, v []string, lcs [][]int) []lcsStep {
	n, m := len(base), len(v)
	var steps []lcsStep
	for i, j := 0, 0; i < n || j < m; {
		switch {
		case i < n && j < m && base[i] == v[j]:
			steps = append(steps, lcsStep{'e', i, j})
			i++
			j++
		case j < m && (i >= n || lcs[i][j+1] >= lcs[i+1][j]):
			steps = append(steps, lcsStep{'i', i, j})
			j++
		default:
			steps = append(steps, lcsStep{'d', i, j})
			i++
		}
	}
	return steps
}

// extendRun folds the consecutive steps of one tag starting at k into
// a [b0,b1) x [v0,v1) run and returns the next unconsumed index.
func extendRun(steps []lcsStep, k int) (tag byte, b0, b1, v0, v1, next int) {
	tag = steps[k].tag
	b0, v0 = steps[k].b, steps[k].v
	b1, v1 = b0, v0
	for next = k; next < len(steps) && steps[next].tag == tag; next++ {
		if tag == 'e' || tag == 'd' {
			b1 = steps[next].b + 1
		} else {
			b1 = steps[next].b
		}
		if tag == 'e' || tag == 'i' {
			v1 = steps[next].v + 1
		} else {
			v1 = steps[next].v
		}
	}
	return
}

// replaceTail folds a delete+insert pair at one base point into a
// single replacement run. It returns the run end and the next index.
func replaceTail(steps []lcsStep, k, b1, v1 int) (b2, v2, next int) {
	b2, v2 = b1, v1
	for ; k < len(steps) && steps[k].tag != 'e' && steps[k].b >= b1; k++ {
		if steps[k].tag == 'd' {
			b2 = steps[k].b + 1
		} else {
			v2 = steps[k].v + 1
		}
	}
	return b2, v2, k
}

// groupOpcodes folds consecutive same-tag steps into runs; a delete
// run abutting an insert run at the same base point is one replacement.
func groupOpcodes(steps []lcsStep) []opcode {
	var ops []opcode
	for k := 0; k < len(steps); {
		t, b0, b1, v0, v1, next := extendRun(steps, k)
		if (t == 'd' || t == 'i') && next < len(steps) && steps[next].tag != 'e' && steps[next].tag != t && steps[next].b == b1 {
			b2, v2, after := replaceTail(steps, next, b1, v1)
			ops = append(ops, opcode{'r', b0, b2, v0, v2})
			k = after
			continue
		}
		ops = append(ops, opcode{t, b0, b1, v0, v1})
		k = next
	}
	return ops
}

func hunkTitle(h lineHunk) string {
	if h.insertLabel {
		return fmt.Sprintf("%s insert at line %d", h.field, h.b0+1)
	}
	return fmt.Sprintf("%s lines %d-%d", h.field, h.b0+1, h.b1)
}

// renderLines renders merged lines with one trailing newline per line;
// an empty value renders as "(empty)".
func renderLines(lines []string) string {
	if len(lines) == 0 {
		return "(empty)\n"
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l + "\n")
	}
	return sb.String()
}

// indentLines renders a conflict side's lines indented two spaces; an
// empty side renders as an indented "(empty)".
func indentLines(lines []string) string {
	if len(lines) == 0 {
		return "    (empty)\n"
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString("    " + l + "\n")
	}
	return sb.String()
}
