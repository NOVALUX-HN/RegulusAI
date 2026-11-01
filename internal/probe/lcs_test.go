package probe

// G4/S4 LCS coverage.
//
// Task: "| G4 | S4 | tests LCS (abutting, bordes, off-by-one) | @regulus-worker |"
// Spec: "### Scenario: S4 LCS cubierto" — `mergeSegments`/`walkClusters`/`lcsTable`/`lcsWalk`/`replaceTail`/`extendRun`
// con casos inserción-abutting-cambio + bordes, `go test ./internal/probe/` verde.
//
// Tests only: no algorithm change.

import (
	"fmt"
	"strings"
	"testing"
)

func mustEqualLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %q (len %d), want %q (len %d)", what, got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: line %d: got %q, want %q (full got %q want %q)", what, i, got[i], want[i], got, want)
		}
	}
}

func mustNoHunks(t *testing.T, what string, hunks []lineHunk) {
	t.Helper()
	if len(hunks) != 0 {
		t.Fatalf("%s: want no hunks, got %+v", what, hunks)
	}
}

// applyOpcodes rebuilds the variant by concatenating variant[v0:v1]
// across ops; every op must also slice the base contiguously.
func applyOpcodes(base, variant []string, ops []opcode) ([]string, []string) {
	var rebuiltV, coveredB []string
	for _, op := range ops {
		coveredB = append(coveredB, base[op.b0:op.b1]...)
		rebuiltV = append(rebuiltV, variant[op.v0:op.v1]...)
	}
	return rebuiltV, coveredB
}

func checkPartition(t *testing.T, what string, base, variant []string, ops []opcode) {
	t.Helper()
	if len(base) == 0 && len(variant) == 0 {
		if len(ops) != 0 {
			t.Fatalf("%s: empty/empty must yield no ops, got %+v", what, ops)
		}
		return
	}
	if len(ops) == 0 {
		t.Fatalf("%s: non-empty pair must yield ops, base=%q variant=%q", what, base, variant)
	}
	for i, op := range ops {
		if op.b0 < 0 || op.b1 < op.b0 || op.b1 > len(base) {
			t.Fatalf("%s: op %d %+v out of base range %d", what, i, op, len(base))
		}
		if op.v0 < 0 || op.v1 < op.v0 || op.v1 > len(variant) {
			t.Fatalf("%s: op %d %+v out of variant range %d", what, i, op, len(variant))
		}
		switch op.tag {
		case 'e':
			if op.b1-op.b0 != op.v1-op.v0 {
				t.Fatalf("%s: op %d %+v: equal run must cover same count", what, i, op)
			}
			for k := 0; k < op.b1-op.b0; k++ {
				if base[op.b0+k] != variant[op.v0+k] {
					t.Fatalf("%s: op %d %+v: equal run mismatches at offset %d", what, i, op, k)
				}
			}
		case 'd':
			if op.v1 != op.v0 {
				t.Fatalf("%s: op %d %+v: delete must carry empty variant range", what, i, op)
			}
		case 'i':
			if op.b1 != op.b0 {
				t.Fatalf("%s: op %d %+v: insert must carry empty base range", what, i, op)
			}
		case 'r':
			if op.b1 <= op.b0 {
				t.Fatalf("%s: op %d %+v: replacement must consume base lines", what, i, op)
			}
		default:
			t.Fatalf("%s: op %d has bad tag %q", what, i, op.tag)
		}
		if i > 0 {
			prev := ops[i-1]
			if op.b0 != prev.b1 || op.v0 != prev.v1 {
				t.Fatalf("%s: ops %d->%d not contiguous: %+v then %+v", what, i-1, i, prev, op)
			}
		}
	}
	if ops[0].b0 != 0 || ops[0].v0 != 0 {
		t.Fatalf("%s: ops must start at 0, got %+v", what, ops[0])
	}
	last := ops[len(ops)-1]
	if last.b1 != len(base) || last.v1 != len(variant) {
		t.Fatalf("%s: ops must end at (%d,%d), got %+v", what, len(base), len(variant), last)
	}
	rebuilt, covered := applyOpcodes(base, variant, ops)
	mustEqualLines(t, what+" rebuild variant", rebuilt, variant)
	mustEqualLines(t, what+" cover base", covered, base)
}

func TestLcsTableEdges(t *testing.T) {
	// Both empty.
	lcs := lcsTable(nil, nil)
	if len(lcs) != 1 || len(lcs[0]) != 1 || lcs[0][0] != 0 {
		t.Fatalf("both empty: want 1x1 zero table, got %v", lcs)
	}
	// Empty variant.
	lcs = lcsTable([]string{"a", "b"}, nil)
	if len(lcs) != 3 || len(lcs[0]) != 1 {
		t.Fatalf("empty variant: want 3x1 table, got %v", lcs)
	}
	if lcs[0][0] != 0 || lcs[1][0] != 0 || lcs[2][0] != 0 {
		t.Fatalf("empty variant: all zeros, got %v", lcs)
	}
	// Empty base.
	lcs = lcsTable(nil, []string{"x"})
	if len(lcs) != 1 || len(lcs[0]) != 2 || lcs[0][0] != 0 || lcs[0][1] != 0 {
		t.Fatalf("empty base: want 1x2 zeros, got %v", lcs)
	}
	// Identical.
	lcs = lcsTable([]string{"l1", "l2", "l3"}, []string{"l1", "l2", "l3"})
	if len(lcs) != 4 || len(lcs[0]) != 4 {
		t.Fatalf("identical: want 4x4 table, got %dx%d", len(lcs), len(lcs[0]))
	}
	if lcs[0][0] != 3 || lcs[1][1] != 2 || lcs[2][2] != 1 || lcs[3][3] != 0 {
		t.Fatalf("identical: bad diagonal %d %d %d %d", lcs[0][0], lcs[1][1], lcs[2][2], lcs[3][3])
	}
	// Totally distinct.
	lcs = lcsTable([]string{"a", "b"}, []string{"x", "y"})
	if lcs[0][0] != 0 {
		t.Fatalf("disjoint: lcs[0][0] = %d, want 0", lcs[0][0])
	}
	// 1-char lines.
	if got := lcsTable([]string{"a"}, []string{"a"})[0][0]; got != 1 {
		t.Fatalf("1-char equal: got %d, want 1", got)
	}
	if got := lcsTable([]string{"a"}, []string{"b"})[0][0]; got != 0 {
		t.Fatalf("1-char distinct: got %d, want 0", got)
	}
	// Prefix: only the head matches.
	if got := lcsTable([]string{"a", "b", "c"}, []string{"a", "x"})[0][0]; got != 1 {
		t.Fatalf("prefix: got %d, want 1", got)
	}
	// Duplicates: ["x","x"] vs ["x"] share one line.
	if got := lcsTable([]string{"x", "x"}, []string{"x"})[0][0]; got != 1 {
		t.Fatalf("duplicates: got %d, want 1", got)
	}
	// Reordered with repeats: LCS length 2.
	if got := lcsTable([]string{"a", "b", "a"}, []string{"a", "a", "b"})[0][0]; got != 2 {
		t.Fatalf("reordered repeats: got %d, want 2", got)
	}
	// Last row/column stay zero.
	lcs = lcsTable([]string{"a", "b"}, []string{"c", "d"})
	for i := 0; i <= 2; i++ {
		if lcs[i][2] != 0 {
			t.Fatalf("last column must be zero, lcs[%d][2]=%d", i, lcs[i][2])
		}
	}
	for j := 0; j <= 2; j++ {
		if lcs[2][j] != 0 {
			t.Fatalf("last row must be zero, lcs[2][%d]=%d", j, lcs[2][j])
		}
	}
}

func TestLcsWalkEdges(t *testing.T) {
	// Both empty: no steps.
	if steps := lcsWalk(nil, nil, lcsTable(nil, nil)); len(steps) != 0 {
		t.Fatalf("both empty: want no steps, got %+v", steps)
	}
	// Empty base: all inserts at point 0, variant order preserved.
	v := []string{"x", "y"}
	steps := lcsWalk(nil, v, lcsTable(nil, v))
	if len(steps) != 2 {
		t.Fatalf("empty base: want 2 inserts, got %+v", steps)
	}
	for k, s := range steps {
		if s.tag != 'i' || s.b != 0 || s.v != k {
			t.Fatalf("empty base step %d: got %+v", k, s)
		}
	}
	// Empty variant: all deletes.
	base := []string{"a", "b"}
	steps = lcsWalk(base, nil, lcsTable(base, nil))
	if len(steps) != 2 {
		t.Fatalf("empty variant: want 2 deletes, got %+v", steps)
	}
	for k, s := range steps {
		if s.tag != 'd' || s.b != k {
			t.Fatalf("empty variant step %d: got %+v", k, s)
		}
	}
	// Identical: all keeps.
	base = []string{"l1", "l2", "l3"}
	steps = lcsWalk(base, base, lcsTable(base, base))
	if len(steps) != 3 {
		t.Fatalf("identical: want 3 keeps, got %+v", steps)
	}
	for k, s := range steps {
		if s.tag != 'e' || s.b != k || s.v != k {
			t.Fatalf("identical step %d: got %+v", k, s)
		}
	}
	// Pure insertion in the middle.
	bl := []string{"l1", "l3"}
	vl := []string{"l1", "l2", "l3"}
	steps = lcsWalk(bl, vl, lcsTable(bl, vl))
	if len(steps) != 3 || steps[0] != (lcsStep{'e', 0, 0}) || steps[1] != (lcsStep{'i', 1, 1}) || steps[2] != (lcsStep{'e', 1, 2}) {
		t.Fatalf("insert middle: got %+v", steps)
	}
	// Pure deletion in the middle.
	bl = []string{"l1", "l2", "l3"}
	vl = []string{"l1", "l3"}
	steps = lcsWalk(bl, vl, lcsTable(bl, vl))
	if len(steps) != 3 || steps[0] != (lcsStep{'e', 0, 0}) || steps[1] != (lcsStep{'d', 1, 1}) || steps[2] != (lcsStep{'e', 2, 1}) {
		t.Fatalf("delete middle: got %+v", steps)
	}
	// Single replacement in the middle folds to insert+delete around keeps.
	bl = []string{"a", "b", "c"}
	vl = []string{"a", "X", "c"}
	steps = lcsWalk(bl, vl, lcsTable(bl, vl))
	if len(steps) != 4 || steps[0] != (lcsStep{'e', 0, 0}) || steps[3] != (lcsStep{'e', 2, 2}) {
		t.Fatalf("replace middle: got %+v", steps)
	}
	if steps[1].tag != 'i' || steps[2].tag != 'd' {
		t.Fatalf("replace middle: middle steps must be insert+delete, got %+v", steps)
	}
	// 1-char total replacement: insert before delete (tie prefers insert).
	steps = lcsWalk([]string{"a"}, []string{"b"}, lcsTable([]string{"a"}, []string{"b"}))
	if len(steps) != 2 || steps[0] != (lcsStep{'i', 0, 0}) || steps[1] != (lcsStep{'d', 0, 1}) {
		t.Fatalf("1-char replace: got %+v", steps)
	}
	// Invariants across shapes: keeps+deletes cover base, keeps+inserts cover variant.
	cases := []struct {
		name string
		b, v []string
	}{
		{"empty-empty", nil, nil},
		{"empty-one", nil, []string{"x"}},
		{"one-empty", []string{"x"}, nil},
		{"identical", []string{"a", "b"}, []string{"a", "b"}},
		{"disjoint", []string{"a", "b"}, []string{"x", "y"}},
		{"dups", []string{"x", "x", "x"}, []string{"x", "y", "x"}},
		{"repeat", []string{"a", "b", "a", "b"}, []string{"a", "a", "b", "b"}},
		{"long", repeatLines("l", 50), append(append([]string{}, repeatLines("l", 24)...), append([]string{"MID"}, repeatLines("l", 25)...)...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := lcsWalk(tc.b, tc.v, lcsTable(tc.b, tc.v))
			baseCovered, varCovered := 0, 0
			for _, s := range st {
				switch s.tag {
				case 'e':
					baseCovered++
					varCovered++
					if tc.b[s.b] != tc.v[s.v] {
						t.Fatalf("keep step %+v mismatches %q vs %q", s, tc.b[s.b], tc.v[s.v])
					}
				case 'd':
					baseCovered++
				case 'i':
					varCovered++
				default:
					t.Fatalf("bad tag %q", s.tag)
				}
			}
			if baseCovered != len(tc.b) || varCovered != len(tc.v) {
				t.Fatalf("walk covers base %d/%d variant %d/%d steps=%+v", baseCovered, len(tc.b), varCovered, len(tc.v), st)
			}
		})
	}
}

func repeatLines(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%03d", prefix, i+1)
	}
	return out
}

func TestExtendRunFolding(t *testing.T) {
	steps := []lcsStep{
		{'e', 0, 0}, {'e', 1, 1},
		{'d', 2, 2}, {'d', 3, 2},
		{'i', 4, 2},
	}
	tag, b0, b1, v0, v1, next := extendRun(steps, 0)
	if tag != 'e' || b0 != 0 || b1 != 2 || v0 != 0 || v1 != 2 || next != 2 {
		t.Fatalf("equal run: got (%q,%d,%d,%d,%d,%d)", tag, b0, b1, v0, v1, next)
	}
	tag, b0, b1, v0, v1, next = extendRun(steps, 2)
	if tag != 'd' || b0 != 2 || b1 != 4 || v0 != 2 || v1 != 2 || next != 4 {
		t.Fatalf("delete run: got (%q,%d,%d,%d,%d,%d)", tag, b0, b1, v0, v1, next)
	}
	tag, b0, b1, v0, v1, next = extendRun(steps, 4)
	if tag != 'i' || b0 != 4 || b1 != 4 || v0 != 2 || v1 != 3 || next != 5 {
		t.Fatalf("insert run: got (%q,%d,%d,%d,%d,%d)", tag, b0, b1, v0, v1, next)
	}
	// Single-step run at the tail.
	tag, b0, b1, v0, v1, next = extendRun([]lcsStep{{'e', 7, 9}}, 0)
	if tag != 'e' || b0 != 7 || b1 != 8 || v0 != 9 || v1 != 10 || next != 1 {
		t.Fatalf("single keep: got (%q,%d,%d,%d,%d,%d)", tag, b0, b1, v0, v1, next)
	}
	// Multi-insert run keeps one base point and spans variant lines.
	multi := []lcsStep{{'i', 2, 5}, {'i', 2, 6}, {'i', 2, 7}, {'e', 2, 8}}
	tag, b0, b1, v0, v1, next = extendRun(multi, 0)
	if tag != 'i' || b0 != 2 || b1 != 2 || v0 != 5 || v1 != 8 || next != 3 {
		t.Fatalf("multi insert: got (%q,%d,%d,%d,%d,%d)", tag, b0, b1, v0, v1, next)
	}
}

func TestReplaceTailFolding(t *testing.T) {
	// Total replacement ["a","b"] -> ["x","y"] walks to i,i,d,d.
	base := []string{"a", "b"}
	variant := []string{"x", "y"}
	steps := lcsWalk(base, variant, lcsTable(base, variant))
	if len(steps) != 4 {
		t.Fatalf("total replace steps: got %+v", steps)
	}
	tag, _, b1, _, v1, next := extendRun(steps, 0)
	if tag != 'i' {
		t.Fatalf("total replace must start with inserts, got %+v", steps)
	}
	b2, v2, after := replaceTail(steps, next, b1, v1)
	if b2 != 2 || v2 != 2 || after != 4 {
		t.Fatalf("replaceTail total: got (%d,%d,%d) steps=%+v", b2, v2, after, steps)
	}
	// Middle replacement stops before the trailing keep.
	base = []string{"a", "b", "c"}
	variant = []string{"a", "X", "c"}
	steps = lcsWalk(base, variant, lcsTable(base, variant))
	tag, _, b1, _, v1, next = extendRun(steps, 1)
	if tag != 'i' {
		t.Fatalf("middle replace must have insert run at 1, got %+v", steps)
	}
	b2, v2, after = replaceTail(steps, next, b1, v1)
	if b2 != 2 || v2 != 2 || after != 3 {
		t.Fatalf("replaceTail middle: got (%d,%d,%d) steps=%+v", b2, v2, after, steps)
	}
	if steps[after].tag != 'e' {
		t.Fatalf("replaceTail must stop at the trailing keep, got %+v", steps)
	}
	// Synthetic delete-then-insert at one base point folds the same way.
	synth := []lcsStep{{'d', 1, 1}, {'i', 2, 1}, {'e', 2, 2}}
	tag, b0, bFirst1, v0, vFirst1, knext := extendRun(synth, 0)
	if tag != 'd' || b0 != 1 || bFirst1 != 2 || v0 != 1 || vFirst1 != 1 || knext != 1 {
		t.Fatalf("synthetic delete run: got (%q,%d,%d,%d,%d,%d)", tag, b0, bFirst1, v0, vFirst1, knext)
	}
	b2, v2, after = replaceTail(synth, knext, bFirst1, vFirst1)
	if b2 != 2 || v2 != 2 || after != 2 {
		t.Fatalf("synthetic replaceTail: got (%d,%d,%d)", b2, v2, after)
	}
}

func TestGroupOpcodesCases(t *testing.T) {
	// Empty/empty: no ops.
	if ops := groupOpcodes(lcsWalk(nil, nil, lcsTable(nil, nil))); len(ops) != 0 {
		t.Fatalf("empty/empty: got %+v", ops)
	}
	// Identical: one equal run.
	ops := diffOpcodes([]string{"l1", "l2", "l3"}, []string{"l1", "l2", "l3"})
	if len(ops) != 1 || ops[0] != (opcode{'e', 0, 3, 0, 3}) {
		t.Fatalf("identical: got %+v", ops)
	}
	// Pure insertion in the middle.
	ops = diffOpcodes([]string{"l1", "l3"}, []string{"l1", "l2", "l3"})
	if len(ops) != 3 || ops[0] != (opcode{'e', 0, 1, 0, 1}) || ops[1] != (opcode{'i', 1, 1, 1, 2}) || ops[2] != (opcode{'e', 1, 2, 2, 3}) {
		t.Fatalf("insert middle: got %+v", ops)
	}
	// Pure deletion in the middle.
	ops = diffOpcodes([]string{"l1", "l2", "l3"}, []string{"l1", "l3"})
	if len(ops) != 3 || ops[0] != (opcode{'e', 0, 1, 0, 1}) || ops[1] != (opcode{'d', 1, 2, 1, 1}) || ops[2] != (opcode{'e', 2, 3, 1, 2}) {
		t.Fatalf("delete middle: got %+v", ops)
	}
	// Single replacement in the middle: insert+delete abut to one 'r'.
	ops = diffOpcodes([]string{"a", "b", "c"}, []string{"a", "X", "c"})
	if len(ops) != 3 || ops[0] != (opcode{'e', 0, 1, 0, 1}) || ops[1] != (opcode{'r', 1, 2, 1, 2}) || ops[2] != (opcode{'e', 2, 3, 2, 3}) {
		t.Fatalf("replace middle: got %+v", ops)
	}
	// Total replacement: one 'r' over everything.
	ops = diffOpcodes([]string{"a", "b"}, []string{"x", "y"})
	if len(ops) != 1 || ops[0] != (opcode{'r', 0, 2, 0, 2}) {
		t.Fatalf("total replace: got %+v", ops)
	}
	// 1-char replacement: one 'r'.
	ops = diffOpcodes([]string{"a"}, []string{"b"})
	if len(ops) != 1 || ops[0] != (opcode{'r', 0, 1, 0, 1}) {
		t.Fatalf("1-char replace: got %+v", ops)
	}
	// Prepend: insert at point 0 then keep.
	ops = diffOpcodes([]string{"b", "c"}, []string{"a", "b", "c"})
	if len(ops) != 2 || ops[0] != (opcode{'i', 0, 0, 0, 1}) || ops[1] != (opcode{'e', 0, 2, 1, 3}) {
		t.Fatalf("prepend: got %+v", ops)
	}
	// Append: keep then insert at point n.
	ops = diffOpcodes([]string{"a", "b"}, []string{"a", "b", "c"})
	if len(ops) != 2 || ops[0] != (opcode{'e', 0, 2, 0, 2}) || ops[1] != (opcode{'i', 2, 2, 2, 3}) {
		t.Fatalf("append: got %+v", ops)
	}
	// Delete head: delete then keep.
	ops = diffOpcodes([]string{"a", "b", "c"}, []string{"b", "c"})
	if len(ops) != 2 || ops[0] != (opcode{'d', 0, 1, 0, 0}) || ops[1] != (opcode{'e', 1, 3, 0, 2}) {
		t.Fatalf("delete head: got %+v", ops)
	}
	// Delete tail: keep then delete.
	ops = diffOpcodes([]string{"a", "b", "c"}, []string{"a", "b"})
	if len(ops) != 2 || ops[0] != (opcode{'e', 0, 2, 0, 2}) || ops[1] != (opcode{'d', 2, 3, 2, 2}) {
		t.Fatalf("delete tail: got %+v", ops)
	}
	// Tail replacement with growth: one 'r' carrying two lines.
	ops = diffOpcodes([]string{"l1", "l2", "l3"}, []string{"l1", "l2", "X", "Y"})
	if len(ops) != 2 || ops[0] != (opcode{'e', 0, 2, 0, 2}) || ops[1] != (opcode{'r', 2, 3, 2, 4}) {
		t.Fatalf("tail replace: got %+v", ops)
	}
	// Synthetic delete-then-insert at one point is one replacement.
	synth := []lcsStep{{'d', 1, 1}, {'i', 2, 1}, {'e', 2, 2}}
	ops = groupOpcodes(synth)
	if len(ops) != 2 || ops[0] != (opcode{'r', 1, 2, 1, 2}) || ops[1] != (opcode{'e', 2, 3, 2, 3}) {
		t.Fatalf("synthetic d+i: got %+v", ops)
	}
}

func TestDiffOpcodesPartitionAndRebuild(t *testing.T) {
	longBase := repeatLines("l", 120)
	longVar := append([]string{}, longBase...)
	longVar[49] = "A-change"
	longVar[99] = "A-change-100"
	cases := []struct {
		name string
		b, v []string
	}{
		{"both-empty", nil, nil},
		{"base-empty", nil, []string{"x", "y"}},
		{"variant-empty", []string{"x", "y"}, nil},
		{"identical", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"disjoint", []string{"a", "b"}, []string{"x", "y"}},
		{"insert-middle", []string{"l1", "l3"}, []string{"l1", "l2", "l3"}},
		{"delete-middle", []string{"l1", "l2", "l3"}, []string{"l1", "l3"}},
		{"replace-middle", []string{"a", "b", "c"}, []string{"a", "X", "c"}},
		{"abutting-pair", []string{"l1", "l2", "l3"}, []string{"A1", "B2", "l3"}},
		{"one-char", []string{"a"}, []string{"b"}},
		{"one-char-equal", []string{"a"}, []string{"a"}},
		{"dups-all-same", []string{"x", "x", "x"}, []string{"x", "y", "x"}},
		{"dups-repeat", []string{"a", "b", "a", "b"}, []string{"a", "a", "b", "b"}},
		{"dups-anchored", []string{"h", "x", "x", "t"}, []string{"h", "x", "y", "x", "t"}},
		{"tail-replace-grow", []string{"l1", "l2", "l3"}, []string{"l1", "l2", "X", "Y"}},
		{"prepend", []string{"b", "c"}, []string{"a", "b", "c"}},
		{"append", []string{"a", "b"}, []string{"a", "b", "c"}},
		{"long-mid-edits", longBase, longVar},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkPartition(t, tc.name, tc.b, tc.v, diffOpcodes(tc.b, tc.v))
		})
	}
}

func TestMergeLineFieldEmpty(t *testing.T) {
	// All empty: nothing merged, no hunks.
	merged, hunks := mergeLineField("stdout", "", "", "")
	if len(merged) != 0 || len(hunks) != 0 {
		t.Fatalf("all empty: got %q %+v", merged, hunks)
	}
	// Base empty, both variants empty: same.
	merged, hunks = mergeLineField("stderr", "", "", "")
	if len(merged) != 0 || len(hunks) != 0 {
		t.Fatalf("stderr all empty: got %q %+v", merged, hunks)
	}
	// Base empty, one side inserts: the insertion wins.
	merged, hunks = mergeLineField("stdout", "", "x", "")
	mustEqualLines(t, "empty base A inserts", merged, []string{"x"})
	mustNoHunks(t, "empty base A inserts", hunks)
	// Base empty, both insert the same: one copy, no conflict.
	merged, hunks = mergeLineField("stdout", "", "x", "x")
	mustEqualLines(t, "empty base same insert", merged, []string{"x"})
	mustNoHunks(t, "empty base same insert", hunks)
	// Base empty, both insert differently at point 0: conflict, insert label.
	merged, hunks = mergeLineField("stdout", "", "x", "y")
	mustEqualLines(t, "empty base conflict keeps base", merged, nil)
	if len(hunks) != 1 {
		t.Fatalf("empty base conflict: want 1 hunk, got %+v", hunks)
	}
	h := hunks[0]
	if h.b0 != 0 || h.b1 != 0 || !h.insertLabel || len(h.base) != 0 {
		t.Fatalf("empty base conflict hunk: got %+v", h)
	}
	mustEqualLines(t, "empty base conflict A", h.a, []string{"x"})
	mustEqualLines(t, "empty base conflict B", h.b, []string{"y"})
	// Both delete everything identically: empty wins, no conflict.
	merged, hunks = mergeLineField("stdout", "a\nb", "", "")
	mustEqualLines(t, "both delete all", merged, nil)
	mustNoHunks(t, "both delete all", hunks)
}

func TestMergeLineFieldIdentical(t *testing.T) {
	merged, hunks := mergeLineField("stdout", "l1\nl2\nl3", "l1\nl2\nl3", "l1\nl2\nl3")
	mustEqualLines(t, "identical", merged, []string{"l1", "l2", "l3"})
	mustNoHunks(t, "identical", hunks)
}

func TestMergeLineFieldTotallyDistinct(t *testing.T) {
	// Both variants replace the whole field differently: base kept, one hunk.
	merged, hunks := mergeLineField("stdout", "a\nb", "x\ny", "p\nq")
	mustEqualLines(t, "totally distinct keeps base", merged, []string{"a", "b"})
	if len(hunks) != 1 {
		t.Fatalf("totally distinct: want 1 hunk, got %+v", hunks)
	}
	h := hunks[0]
	if h.field != "stdout" || h.b0 != 0 || h.b1 != 2 || h.insertLabel {
		t.Fatalf("totally distinct hunk header: got %+v", h)
	}
	mustEqualLines(t, "totally distinct base", h.base, []string{"a", "b"})
	mustEqualLines(t, "totally distinct A", h.a, []string{"x", "y"})
	mustEqualLines(t, "totally distinct B", h.b, []string{"p", "q"})
}

func TestMergeLineFieldPureInsert(t *testing.T) {
	// A inserts, B unchanged: insertion wins.
	merged, hunks := mergeLineField("stdout", "l1\nl3", "l1\nl2\nl3", "l1\nl3")
	mustEqualLines(t, "pure insert takes A", merged, []string{"l1", "l2", "l3"})
	mustNoHunks(t, "pure insert takes A", hunks)
	// Both insert the same line at the same point: one copy, no conflict.
	merged, hunks = mergeLineField("stdout", "l1\nl3", "l1\nX\nl3", "l1\nX\nl3")
	mustEqualLines(t, "same insert", merged, []string{"l1", "X", "l3"})
	mustNoHunks(t, "same insert", hunks)
	// Different inserts at the same point: conflict, base kept, insert label.
	merged, hunks = mergeLineField("stdout", "l1\nl2", "l1\nX\nl2", "l1\nY\nl2")
	mustEqualLines(t, "insert conflict keeps base", merged, []string{"l1", "l2"})
	if len(hunks) != 1 {
		t.Fatalf("insert conflict: want 1 hunk, got %+v", hunks)
	}
	h := hunks[0]
	if h.b0 != 1 || h.b1 != 1 || !h.insertLabel {
		t.Fatalf("insert conflict hunk must mark point 1, got %+v", h)
	}
	if len(h.base) != 0 {
		t.Fatalf("insert conflict base must be empty, got %+v", h)
	}
	mustEqualLines(t, "insert conflict A", h.a, []string{"X"})
	mustEqualLines(t, "insert conflict B", h.b, []string{"Y"})
	if got := hunkTitle(h); got != "stdout insert at line 2" {
		t.Fatalf("insert title = %q, want %q", got, "stdout insert at line 2")
	}
}

func TestMergeLineFieldPureDelete(t *testing.T) {
	// A deletes one line, B unchanged: deletion wins.
	merged, hunks := mergeLineField("stdout", "l1\nl2\nl3", "l1\nl3", "l1\nl2\nl3")
	mustEqualLines(t, "pure delete", merged, []string{"l1", "l3"})
	mustNoHunks(t, "pure delete", hunks)
	// Both delete the same line: deletion wins once, no conflict.
	merged, hunks = mergeLineField("stdout", "l1\nl2\nl3", "l1\nl3", "l1\nl3")
	mustEqualLines(t, "same delete", merged, []string{"l1", "l3"})
	mustNoHunks(t, "same delete", hunks)
	// Overlapping deletes of different extent: conflict, base kept, empty sides.
	merged, hunks = mergeLineField("stdout", "l1\nl2\nl3\nl4", "l1\nl3\nl4", "l1\nl4")
	mustEqualLines(t, "overlapping deletes keep base", merged, []string{"l1", "l2", "l3", "l4"})
	if len(hunks) != 1 {
		t.Fatalf("overlapping deletes: want 1 hunk, got %+v", hunks)
	}
	h := hunks[0]
	if h.b0 != 1 || h.b1 != 3 || h.insertLabel {
		t.Fatalf("overlapping deletes hunk range: got %+v", h)
	}
	mustEqualLines(t, "overlapping deletes base", h.base, []string{"l2", "l3"})
	if len(h.a) != 0 || len(h.b) != 0 {
		t.Fatalf("overlapping deletes sides must be empty, got %+v", h)
	}
}

func TestMergeLineFieldAbuttingChangesMerge(t *testing.T) {
	// Fin de uno = inicio de otro: A owns [0,1), B owns [1,2). Both apply.
	merged, hunks := mergeLineField("stdout", "l1\nl2\nl3\nl4", "A1\nl2\nl3\nl4", "l1\nB2\nl3\nl4")
	mustEqualLines(t, "abutting pair", merged, []string{"A1", "B2", "l3", "l4"})
	mustNoHunks(t, "abutting pair", hunks)
	// Interleaved abutting edits on both sides all apply, in base order.
	merged, hunks = mergeLineField("stdout", "l1\nl2\nl3\nl4", "A1\nl2\nA3\nl4", "l1\nB2\nl3\nB4")
	mustEqualLines(t, "abutting interleave", merged, []string{"A1", "B2", "A3", "B4"})
	mustNoHunks(t, "abutting interleave", hunks)
	// Abutting at the tail: A owns [2,3), B owns [3,4).
	merged, hunks = mergeLineField("stdout", "l1\nl2\nl3\nl4", "l1\nl2\nA3\nl4", "l1\nl2\nl3\nB4")
	mustEqualLines(t, "abutting tail", merged, []string{"l1", "l2", "A3", "B4"})
	mustNoHunks(t, "abutting tail", hunks)
}

func TestMergeLineFieldAbuttingInsertAndChangeMerge(t *testing.T) {
	// Insertion at point 1 merely abuts the change starting at 1: both apply.
	merged, hunks := mergeLineField("stdout", "l1\nl2", "l1\nX\nl2", "l1\nY")
	mustEqualLines(t, "insert abuts change", merged, []string{"l1", "X", "Y"})
	mustNoHunks(t, "insert abuts change", hunks)
	// Same geometry with sides swapped: the insert still sorts first, same merge.
	merged, hunks = mergeLineField("stdout", "l1\nl2", "l1\nY", "l1\nX\nl2")
	mustEqualLines(t, "change abuts insert", merged, []string{"l1", "X", "Y"})
	mustNoHunks(t, "change abuts insert", hunks)
}

func TestMergeLineFieldOverlappingChangesConflict(t *testing.T) {
	// Same base line replaced differently: base kept, exact hunk.
	merged, hunks := mergeLineField("stdout", "l1\nl2\nl3", "l1\nA2\nl3", "l1\nB2\nl3")
	mustEqualLines(t, "overlap keeps base", merged, []string{"l1", "l2", "l3"})
	if len(hunks) != 1 {
		t.Fatalf("overlap: want 1 hunk, got %+v", hunks)
	}
	h := hunks[0]
	if h.field != "stdout" || h.b0 != 1 || h.b1 != 2 || h.insertLabel {
		t.Fatalf("overlap hunk header: got %+v", h)
	}
	mustEqualLines(t, "overlap base", h.base, []string{"l2"})
	mustEqualLines(t, "overlap A", h.a, []string{"A2"})
	mustEqualLines(t, "overlap B", h.b, []string{"B2"})
	if got := hunkTitle(h); got != "stdout lines 2-2" {
		t.Fatalf("overlap title = %q, want %q", got, "stdout lines 2-2")
	}
	// Strict containment also conflicts: A replaces [0,3), B replaces [1,2).
	merged, hunks = mergeLineField("stdout", "l1\nl2\nl3", "A1\nA2\nA3", "l1\nB2\nl3")
	mustEqualLines(t, "containment keeps base", merged, []string{"l1", "l2", "l3"})
	if len(hunks) != 1 {
		t.Fatalf("containment: want 1 hunk, got %+v", hunks)
	}
	if hunks[0].b0 != 0 || hunks[0].b1 != 3 {
		t.Fatalf("containment hunk must span 0-3, got %+v", hunks[0])
	}
}

func TestMergeLineFieldSingleChar(t *testing.T) {
	// One 1-char line replaced differently: conflict keeps base.
	merged, hunks := mergeLineField("stdout", "a", "b", "c")
	mustEqualLines(t, "1-char conflict keeps base", merged, []string{"a"})
	if len(hunks) != 1 {
		t.Fatalf("1-char conflict: want 1 hunk, got %+v", hunks)
	}
	mustEqualLines(t, "1-char base", hunks[0].base, []string{"a"})
	mustEqualLines(t, "1-char A", hunks[0].a, []string{"b"})
	mustEqualLines(t, "1-char B", hunks[0].b, []string{"c"})
	// Same 1-char replacement on both sides: takes it, no conflict.
	merged, hunks = mergeLineField("stdout", "a", "b", "b")
	mustEqualLines(t, "1-char same", merged, []string{"b"})
	mustNoHunks(t, "1-char same", hunks)
	// 1-char insert after one line: takes the insertion.
	merged, hunks = mergeLineField("stdout", "a", "a\nb", "a")
	mustEqualLines(t, "1-char append", merged, []string{"a", "b"})
	mustNoHunks(t, "1-char append", hunks)
}

func TestMergeLineFieldDuplicateLines(t *testing.T) {
	// Identical edits over repeated lines apply once, no conflict.
	merged, hunks := mergeLineField("stdout", "x\nx\nx", "x\ny\nx\nx", "x\ny\nx\nx")
	mustEqualLines(t, "dups identical", merged, []string{"x", "y", "x", "x"})
	mustNoHunks(t, "dups identical", hunks)
	// Anchored duplicates: insert into the same x-gap on one side wins.
	merged, hunks = mergeLineField("stdout", "h\nx\nx\nt", "h\nx\ny\nx\nt", "h\nx\nx\nt")
	mustEqualLines(t, "dups anchored insert", merged, []string{"h", "x", "y", "x", "t"})
	mustNoHunks(t, "dups anchored insert", hunks)
	// Anchored duplicates: different inserts into the same gap conflict.
	merged, hunks = mergeLineField("stdout", "h\nx\nx\nt", "h\nx\ny\nx\nt", "h\nx\nz\nx\nt")
	mustEqualLines(t, "dups anchored conflict keeps base", merged, []string{"h", "x", "x", "t"})
	if len(hunks) != 1 {
		t.Fatalf("dups anchored conflict: want 1 hunk, got %+v", hunks)
	}
	h := hunks[0]
	if !h.insertLabel || len(h.base) != 0 {
		t.Fatalf("dups anchored conflict must be a pure insertion, got %+v", h)
	}
	mustEqualLines(t, "dups anchored A", h.a, []string{"y"})
	mustEqualLines(t, "dups anchored B", h.b, []string{"z"})
	// Fully repeated base still partitions and rebuilds on each side.
	for _, tc := range [][2]string{
		{"x\nx\nx\nx", "x\ny\nx\nx"},
		{"a\nb\na\nb", "a\na\nb\nb"},
	} {
		bl := strings.Split(tc[0], "\n")
		vl := strings.Split(tc[1], "\n")
		checkPartition(t, "dups rebuild "+tc[0], bl, vl, diffOpcodes(bl, vl))
	}
}

func TestMergeLineFieldReplaceTail(t *testing.T) {
	// Tail replacement on one side wins.
	merged, hunks := mergeLineField("stdout", "l1\nl2\nl3", "l1\nl2\nX\nY", "l1\nl2\nl3")
	mustEqualLines(t, "tail replace takes A", merged, []string{"l1", "l2", "X", "Y"})
	mustNoHunks(t, "tail replace takes A", hunks)
	// Different tail replacements conflict over the tail range.
	merged, hunks = mergeLineField("stdout", "l1\nl2\nl3", "l1\nl2\nX", "l1\nl2\nY")
	mustEqualLines(t, "tail conflict keeps base", merged, []string{"l1", "l2", "l3"})
	if len(hunks) != 1 {
		t.Fatalf("tail conflict: want 1 hunk, got %+v", hunks)
	}
	h := hunks[0]
	if h.b0 != 2 || h.b1 != 3 || h.insertLabel {
		t.Fatalf("tail conflict hunk range: got %+v", h)
	}
	mustEqualLines(t, "tail base", h.base, []string{"l3"})
	mustEqualLines(t, "tail A", h.a, []string{"X"})
	mustEqualLines(t, "tail B", h.b, []string{"Y"})
	if got := hunkTitle(h); got != "stdout lines 3-3" {
		t.Fatalf("tail title = %q, want %q", got, "stdout lines 3-3")
	}
	// Appends at point n: one side wins; different appends conflict as inserts.
	merged, hunks = mergeLineField("stdout", "l1\nl2", "l1\nl2\nX", "l1\nl2")
	mustEqualLines(t, "append takes A", merged, []string{"l1", "l2", "X"})
	mustNoHunks(t, "append takes A", hunks)
	merged, hunks = mergeLineField("stdout", "l1\nl2", "l1\nl2\nX", "l1\nl2\nY")
	mustEqualLines(t, "append conflict keeps base", merged, []string{"l1", "l2"})
	if len(hunks) != 1 || !hunks[0].insertLabel || hunks[0].b0 != 2 || hunks[0].b1 != 2 {
		t.Fatalf("append conflict must insert at 2, got %+v", hunks)
	}
	if got := hunkTitle(hunks[0]); got != "stdout insert at line 3" {
		t.Fatalf("append title = %q, want %q", got, "stdout insert at line 3")
	}
	// Tail deletion on one side wins.
	merged, hunks = mergeLineField("stdout", "l1\nl2\nl3", "l1\nl2", "l1\nl2\nl3")
	mustEqualLines(t, "tail delete", merged, []string{"l1", "l2"})
	mustNoHunks(t, "tail delete", hunks)
}

func TestMergeLineFieldBordersOffByOne(t *testing.T) {
	// Insert at point 0 wins; different inserts at 0 conflict as line 1.
	merged, hunks := mergeLineField("stdout", "b\nc", "a\nb\nc", "b\nc")
	mustEqualLines(t, "prepend takes A", merged, []string{"a", "b", "c"})
	mustNoHunks(t, "prepend takes A", hunks)
	merged, hunks = mergeLineField("stdout", "b\nc", "a\nb\nc", "z\nb\nc")
	mustEqualLines(t, "prepend conflict keeps base", merged, []string{"b", "c"})
	if len(hunks) != 1 || !hunks[0].insertLabel || hunks[0].b0 != 0 {
		t.Fatalf("prepend conflict must insert at 0, got %+v", hunks)
	}
	if got := hunkTitle(hunks[0]); got != "stdout insert at line 1" {
		t.Fatalf("prepend title = %q, want %q", got, "stdout insert at line 1")
	}
	// Delete head wins.
	merged, hunks = mergeLineField("stdout", "a\nb\nc", "b\nc", "a\nb\nc")
	mustEqualLines(t, "delete head", merged, []string{"b", "c"})
	mustNoHunks(t, "delete head", hunks)
	// Delete head + delete tail are disjoint: both apply, middle survives.
	merged, hunks = mergeLineField("stdout", "a\nb\nc", "b\nc", "a\nb")
	mustEqualLines(t, "delete both ends", merged, []string{"b"})
	mustNoHunks(t, "delete both ends", hunks)
	// Replace head on one side wins.
	merged, hunks = mergeLineField("stdout", "a\nb\nc", "X\nb\nc", "a\nb\nc")
	mustEqualLines(t, "replace head", merged, []string{"X", "b", "c"})
	mustNoHunks(t, "replace head", hunks)
}

func TestMergeLineFieldLongRuns(t *testing.T) {
	base := repeatLines("l", 200)
	baseStr := strings.Join(base, "\n")
	aLines := append([]string{}, base...)
	aLines[49] = "A-change-050"
	bLines := append([]string{}, base...)
	bLines[149] = "B-change-150"
	merged, hunks := mergeLineField("stdout", baseStr, strings.Join(aLines, "\n"), strings.Join(bLines, "\n"))
	mustNoHunks(t, "long disjoint", hunks)
	if len(merged) != 200 || merged[49] != "A-change-050" || merged[149] != "B-change-150" {
		t.Fatalf("long disjoint: bad merge at 50/150, len=%d", len(merged))
	}
	if merged[0] != "l001" || merged[199] != "l200" {
		t.Fatalf("long disjoint: borders moved: %q .. %q", merged[0], merged[199])
	}
	// Same long line changed differently conflicts and keeps the base line.
	aLines2 := append([]string{}, base...)
	aLines2[99] = "A-mid"
	bLines2 := append([]string{}, base...)
	bLines2[99] = "B-mid"
	merged, hunks = mergeLineField("stdout", baseStr, strings.Join(aLines2, "\n"), strings.Join(bLines2, "\n"))
	mustEqualLines(t, "long conflict keeps base line", merged[99:100], []string{"l100"})
	if len(hunks) != 1 {
		t.Fatalf("long conflict: want 1 hunk, got %d", len(hunks))
	}
	h := hunks[0]
	if h.b0 != 99 || h.b1 != 100 {
		t.Fatalf("long conflict hunk range: got %+v", h)
	}
	mustEqualLines(t, "long conflict A", h.a, []string{"A-mid"})
	mustEqualLines(t, "long conflict B", h.b, []string{"B-mid"})
}

func TestWalkClustersDirect(t *testing.T) {
	bl := []string{"l1", "l2", "l3", "l4"}
	// Single edit lands alone.
	merged, hunks := walkClusters("stdout", bl, []seg{{b0: 1, b1: 2, lines: []string{"X"}, fromA: true}})
	mustEqualLines(t, "single", merged, []string{"l1", "X", "l3", "l4"})
	mustNoHunks(t, "single", hunks)
	// Identical edits on both sides land once, no conflict.
	merged, hunks = walkClusters("stdout", bl, []seg{
		{b0: 1, b1: 2, lines: []string{"X"}, fromA: true},
		{b0: 1, b1: 2, lines: []string{"X"}, fromA: false},
	})
	mustEqualLines(t, "identical pair", merged, []string{"l1", "X", "l3", "l4"})
	mustNoHunks(t, "identical pair", hunks)
	// Same range, different lines: conflict keeps base.
	merged, hunks = walkClusters("stdout", bl, []seg{
		{b0: 1, b1: 2, lines: []string{"A"}, fromA: true},
		{b0: 1, b1: 2, lines: []string{"B"}, fromA: false},
	})
	mustEqualLines(t, "same range conflict", merged, bl)
	if len(hunks) != 1 || hunks[0].b0 != 1 || hunks[0].b1 != 2 {
		t.Fatalf("same range conflict: got %+v", hunks)
	}
	// Strict overlap with different extents conflicts over the union.
	merged, hunks = walkClusters("stdout", bl, []seg{
		{b0: 0, b1: 3, lines: []string{"A"}, fromA: true},
		{b0: 1, b1: 2, lines: []string{"B"}, fromA: false},
	})
	mustEqualLines(t, "overlap union keeps base", merged, bl)
	if len(hunks) != 1 || hunks[0].b0 != 0 || hunks[0].b1 != 3 {
		t.Fatalf("overlap union: got %+v", hunks)
	}
	mustEqualLines(t, "overlap union base", hunks[0].base, []string{"l1", "l2", "l3"})
	// Abutting replacements (fin de uno = inicio de otro) stay separate.
	merged, hunks = walkClusters("stdout", bl, []seg{
		{b0: 0, b1: 1, lines: []string{"A1"}, fromA: true},
		{b0: 1, b1: 2, lines: []string{"B2"}, fromA: false},
	})
	mustEqualLines(t, "abutting separate", merged, []string{"A1", "B2", "l3", "l4"})
	mustNoHunks(t, "abutting separate", hunks)
	// Insertion abutting a change stays separate: both apply.
	merged, hunks = walkClusters("stdout", []string{"l1", "l2"}, []seg{
		{b0: 1, b1: 1, lines: []string{"X"}, fromA: true},
		{b0: 1, b1: 2, lines: []string{"Y"}, fromA: false},
	})
	mustEqualLines(t, "insert abuts change", merged, []string{"l1", "X", "Y"})
	mustNoHunks(t, "insert abuts change", hunks)
	// Two different inserts at one point conflict with the insert label.
	merged, hunks = walkClusters("stdout", []string{"l1", "l2"}, []seg{
		{b0: 1, b1: 1, lines: []string{"X"}, fromA: true},
		{b0: 1, b1: 1, lines: []string{"Y"}, fromA: false},
	})
	mustEqualLines(t, "insert-insert keeps base", merged, []string{"l1", "l2"})
	if len(hunks) != 1 || !hunks[0].insertLabel || hunks[0].b0 != 1 || hunks[0].b1 != 1 {
		t.Fatalf("insert-insert: got %+v", hunks)
	}
	// Two identical inserts at one point apply once.
	merged, hunks = walkClusters("stdout", []string{"l1", "l2"}, []seg{
		{b0: 1, b1: 1, lines: []string{"X"}, fromA: true},
		{b0: 1, b1: 1, lines: []string{"X"}, fromA: false},
	})
	mustEqualLines(t, "identical inserts", merged, []string{"l1", "X", "l2"})
	mustNoHunks(t, "identical inserts", hunks)
	// Inserts at different points stay separate.
	merged, hunks = walkClusters("stdout", []string{"l1", "l2"}, []seg{
		{b0: 0, b1: 0, lines: []string{"A"}, fromA: true},
		{b0: 2, b1: 2, lines: []string{"B"}, fromA: false},
	})
	mustEqualLines(t, "inserts apart", merged, []string{"A", "l1", "l2", "B"})
	mustNoHunks(t, "inserts apart", hunks)
}

func TestMergeSegmentsSortsAndMerges(t *testing.T) {
	// A edits the tail, B edits the head: sorted walk merges both ends.
	bl := []string{"l1", "l2", "l3", "l4"}
	merged, hunks := mergeSegments("stdout", bl,
		[]string{"l1", "l2", "l3", "B4"},
		[]string{"A1", "l2", "l3", "l4"})
	mustEqualLines(t, "ends merge", merged, []string{"A1", "l2", "l3", "B4"})
	mustNoHunks(t, "ends merge", hunks)
	// Overlapping edits conflict regardless of which side is head/tail.
	merged, hunks = mergeSegments("stdout",
		[]string{"l1", "l2", "l3"},
		[]string{"l1", "B2", "l3"},
		[]string{"A1", "A2", "A3"})
	mustEqualLines(t, "overlap keeps base", merged, []string{"l1", "l2", "l3"})
	if len(hunks) != 1 || hunks[0].b0 != 0 || hunks[0].b1 != 3 {
		t.Fatalf("overlap: got %+v", hunks)
	}
}
