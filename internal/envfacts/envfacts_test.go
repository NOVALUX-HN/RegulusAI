package envfacts

// M4 miner tests: fixtures of receipts and traces on a temp root prove
// the deterministic rules — ≥3 sessions mines, a clean recent window
// discards, re-mining never duplicates, unattributed data never forms
// domain facts, and mined facts surface through the existing
// per-domain lesson search (the session-start injection path).

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

func writeChart(t *testing.T, root, chart, domain string) {
	t.Helper()
	body := "# " + chart + "\n"
	if domain != "" {
		body += "Domain: " + domain + "\n"
	}
	if err := os.MkdirAll(filepath.Join(root, "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts", chart+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeReceipt(t *testing.T, root, task, gate, verdict, runID string, at time.Time) string {
	t.Helper()
	p, err := receipts.Write(evidence.TaskDir(root, task), receipts.Receipt{
		Task: task, Gate: gate, Verdict: verdict, RunID: runID, StartedAt: at,
		Command: "cmd", ExitCode: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func recordTrace(t *testing.T, root, session, tool, status, task string, at time.Time) {
	t.Helper()
	if _, err := trace.Record(root, trace.Entry{
		Session: session, Message: "m-" + session + "-" + tool, Call: "c-" + session + "-" + tool,
		Tool: tool, Status: status, Task: task,
	}, at); err != nil {
		t.Fatal(err)
	}
}

func scanAll(t *testing.T, root string) []Series {
	t.Helper()
	rs, _, err := ReceiptSeries(root)
	if err != nil {
		t.Fatal(err)
	}
	ts, _, err := TraceSeries(root)
	if err != nil {
		t.Fatal(err)
	}
	return append(rs, ts...)
}

func TestMineGateFactAfterThreeFailingRuns(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	decisions, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Action != ActionMine {
		t.Fatalf("three failing runs must plan one mine, got %+v", decisions)
	}
	results, err := Apply(root, decisions, base.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if results[0].FactID == "" {
		t.Fatal("apply must record the mined fact id")
	}
	// The fact must flow to the existing per-domain lesson injection:
	// `memory search --type lesson --domain`.
	hits, err := almanac.Search(root, almanac.Query{Type: almanac.TypeLesson, Domain: "sextant"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != results[0].FactID {
		t.Fatalf("mined fact must surface via domain lesson search, got %+v", hits)
	}
	if hits[0].TopicKey != "envfact/sextant/gate/crap" {
		t.Fatalf("fact topic must namespace the key, got %q", hits[0].TopicKey)
	}
	if hits[0].Lesson == nil || len(hits[0].Lesson.Evidence) != 3 {
		t.Fatalf("fact must cite the three failing receipts, got %+v", hits[0].Lesson)
	}
}

func TestContradictedCandidateNeverMinedIsSkipped(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_f%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictPass,
			fmt.Sprintf("run_p%d", i), base.Add(time.Duration(10+i)*time.Hour))
	}
	decisions, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Action != ActionSkip {
		t.Fatalf("clean recent window must skip the contradicted candidate, got %+v", decisions)
	}
	results, err := Apply(root, decisions, base.Add(20*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if results[0].FactID != "" || results[0].NoteID != "" {
		t.Fatalf("skip must write nothing, got %+v", results[0])
	}
	all, err := almanac.All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("contradicted candidate must leave the log empty, got %+v", all)
	}
}

func TestContradictedLiveFactIsDiscardedAndHidden(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_f%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	decisions, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, decisions, base.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictPass,
			fmt.Sprintf("run_p%d", i), base.Add(time.Duration(10+i)*time.Hour))
	}
	decisions, err = Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Action != ActionDiscard {
		t.Fatalf("contradicted live fact must plan a discard, got %+v", decisions)
	}
	results, err := Apply(root, decisions, base.Add(20*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if results[0].NoteID == "" {
		t.Fatal("discard must record the contradiction note")
	}
	// Discarded facts hide from the domain injection but resurface stale.
	hits, err := almanac.Search(root, almanac.Query{Type: almanac.TypeLesson, Domain: "sextant"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.TopicKey == "envfact/sextant/gate/crap" && h.Type == almanac.TypeLesson {
			t.Fatalf("discarded fact must hide from default retrieval, got %+v", hits)
		}
	}
	stale, err := almanac.Search(root, almanac.Query{Type: almanac.TypeLesson, Domain: "sextant", IncludeStale: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range stale {
		if h.TopicKey == "envfact/sextant/gate/crap" && h.Type == almanac.TypeLesson {
			found = true
		}
	}
	if !found {
		t.Fatal("discarded fact must resurface with --include-stale (reversible)")
	}
}

func TestRecurrenceReMinesAfterDiscard(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_a%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	first, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, first, base.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictPass,
			fmt.Sprintf("run_b%d", i), base.Add(time.Duration(10+i)*time.Hour))
	}
	second, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, second, base.Add(20*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// The failure recurs: three new failing runs after the clean window.
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_c%d", i), base.Add(time.Duration(30+i)*time.Hour))
	}
	third, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 1 || third[0].Action != ActionMine {
		t.Fatalf("recurring failure must re-mine after discard, got %+v", third)
	}
	now := base.Add(40 * time.Hour)
	results, err := Apply(root, third, now)
	if err != nil {
		t.Fatal(err)
	}
	chain, err := almanac.History(root, "envfact/sextant/gate/crap")
	if err != nil {
		t.Fatal(err)
	}
	lessons := 0
	for _, o := range chain {
		if o.Type == almanac.TypeLesson {
			lessons++
		}
	}
	if lessons != 2 {
		t.Fatalf("re-mine must append a second revision to the same topic, got %d lessons (%s)", lessons, results[0].FactID)
	}
}

func TestTraceToolErrorsMinePerDomain(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		sess := fmt.Sprintf("ses_trace%d", i)
		recordTrace(t, root, sess, "read", trace.StatusError, "feat/T1", base.Add(time.Duration(i)*time.Hour))
		recordTrace(t, root, sess, "read", trace.StatusCompleted, "feat/T1", base.Add(time.Duration(i)*time.Hour+time.Minute))
	}
	decisions, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Action != ActionMine {
		t.Fatalf("tool erroring in three sessions must mine, got %+v", decisions)
	}
	if decisions[0].Series.Key.Kind != KindTool || decisions[0].Series.Key.Name != "read" {
		t.Fatalf("wrong series key, got %+v", decisions[0].Series.Key)
	}
}

func TestUnattributedDataNeverFormsDomainFacts(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	// Task with a chart but no Domain line, plus a task with no chart.
	writeChart(t, root, "plain", "")
	for i := 0; i < 4; i++ {
		writeReceipt(t, root, "plain/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_x%d", i), base.Add(time.Duration(i)*time.Hour))
		writeReceipt(t, root, "ghost/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_y%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	rs, st, err := ReceiptSeries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 0 {
		t.Fatalf("unattributed receipts must form no series, got %+v", rs)
	}
	if st.ReceiptsUnattributed != 8 {
		t.Fatalf("all eight receipts lack a domain, got %+v", st)
	}
	// Trace entries without a task predate attribution and are skipped.
	recordTrace(t, root, "ses_old", "read", trace.StatusError, "", base)
	_, tst, err := TraceSeries(root)
	if err != nil {
		t.Fatal(err)
	}
	if tst.TraceUnattributed != 1 {
		t.Fatalf("taskless trace entries must count as unattributed, got %+v", tst)
	}
}

func TestLegacyReceiptsWithoutRunIDCountAsSessions(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "typecheck", receipts.VerdictFail,
			"", base.Add(time.Duration(i)*time.Hour))
	}
	series := scanAll(t, root)
	if len(series) != 1 {
		t.Fatalf("expected one series, got %+v", series)
	}
	if got := FailingSessions(series[0]); got != 3 {
		t.Fatalf("legacy receipts must count as three distinct executions, got %d", got)
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	a, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%+v", a) != fmt.Sprintf("%+v", b) {
		t.Fatalf("planning must be deterministic:\n%+v\n%+v", a, b)
	}
}

func TestContradictionEdges(t *testing.T) {
	if IsContradicted(Series{}) {
		t.Fatal("empty series is not contradicted")
	}
	if IsCandidate(Series{}) {
		t.Fatal("empty series is not a candidate")
	}
	two := Series{Execs: []Execution{
		{Session: "a", Failed: true}, {Session: "b", Failed: true},
	}}
	if IsCandidate(two) {
		t.Fatal("two failing sessions are not enough")
	}
	if IsContradicted(two) {
		t.Fatal("two failures are not a clean window")
	}
	clean := Series{Execs: []Execution{
		{Session: "a", Failed: true}, {Session: "b", Failed: true},
		{Session: "c", Failed: false},
	}}
	if IsContradicted(clean) {
		t.Fatal("a window holding a failure is not contradicted")
	}
	// H4: confidence is fixed at medium; topics are slugified.
	if (Key{Kind: KindGate, Name: "a/b", Domain: "sextant"}).Topic() != "envfact/sextant/gate/a-b" {
		t.Fatalf("topic must slugify unsafe chars, got %q", (Key{Kind: KindGate, Name: "a/b", Domain: "sextant"}).Topic())
	}
	if (Key{Kind: KindTool, Name: "", Domain: "x y"}).Topic() != "envfact/x-y/tool/unknown" {
		t.Fatalf("empty segments must fall back to unknown, got %q", (Key{Kind: KindTool, Name: "", Domain: "x y"}).Topic())
	}
}
