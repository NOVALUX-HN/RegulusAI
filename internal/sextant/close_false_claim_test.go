package sextant

import (
	"os"
	"strings"
	"testing"
	"time"
)

// forgedChart is a chart that LIES: the task row says done and the Log carries a
// CLOSED line claiming the gauntlet passed. Every test here puts contradicting
// evidence underneath and asserts that close still refuses.
//
// This is the false-claim dimension of F20.4. Failing evidence already blocked clean
// charts, and a lying status line was only caught by CheckProgress - never by the
// evidence-backed close. "Close cannot be lied into" was asserted in prose and not
// executed by any test.
func forgedChart() string {
	doc := closeChart("STANDARD", "")
	doc = strings.Replace(doc,
		"| T1 | the happy path | build | @regulus-worker | - | doing |",
		"| T1 | the happy path | build | @regulus-worker | - | done |", 1)
	doc += "- CLOSED T1: gauntlet 6/6 PASS, verified independently\n"
	return doc
}

func TestForgedCloseWithoutEvidenceIsRefused(t *testing.T) {
	root := t.TempDir()
	res := CheckClose(forgedChart(), root, "feat", "T1")
	if res.Closed {
		t.Fatal("a chart that claims done with no evidence must not close")
	}
	if len(res.Unmet) == 0 {
		t.Fatal("refusal must name why, got no unmet items")
	}
}

func TestForgedCloseWithFailingEvidenceIsRefused(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writeManifestRun(t, root, "feat/T1", "run_forgedfail01", at, []string{"build"}, "fail", 1)

	res := CheckClose(forgedChart(), root, "feat", "T1")
	if res.Closed {
		t.Fatal("a failing gate must not close, however loudly the chart claims it passed")
	}
	joined := strings.Join(res.Unmet, "\n")
	if !strings.Contains(joined, "build") {
		t.Fatalf("refusal must name the failing gate, got %v", res.Unmet)
	}
}

func TestForgedCloseWithTamperedEvidenceIsRefused(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	_, paths := writeManifestRun(t, root, "feat/T1", "run_forgedtamper", at, []string{"build"}, "pass", 0)

	// Rewrite the receipt body: the manifest still lists the original hash, so the
	// evidence chain breaks even though every verdict field still says PASS.
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "ok-build", "ok-forged", 1)
	if tampered == string(data) {
		t.Fatal("fixture did not change the receipt: the test would pass vacuously")
	}
	if err := os.WriteFile(paths[0], []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	res := CheckClose(forgedChart(), root, "feat", "T1")
	if res.Closed {
		t.Fatal("a tampered receipt must not close, however intact its PASS fields look")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "verify") {
		t.Fatalf("refusal must name the evidence verification, got %v", res.Unmet)
	}
}

// A forged claim must not become a way in through the short circuit either: TRIVIAL
// charts close without evidence by design, so the fixture has to prove that the lie
// is what is being judged, not the class.
func TestForgedClaimIsJudgedByEvidenceNotByStatusText(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writeManifestRun(t, root, "feat/T1", "run_forgedgood01", at, []string{"build"}, "pass", 0)

	honest := closeChart("STANDARD", "")
	honest = strings.Replace(honest,
		"| T1 | the happy path | build | @regulus-worker | - | doing |",
		"| T1 | the happy path | build | @regulus-worker | - | done |", 1)
	honest += "- CLOSED T1: closed.\n"

	if res := CheckClose(honest, root, "feat", "T1"); !res.Closed {
		t.Fatalf("the same claim with real evidence must close, got %v", res.Unmet)
	}
	if res := CheckClose(forgedChart(), root, "feat", "T1"); !res.Closed {
		t.Fatalf("the forged chart differs only in wording, so it must also close: %v", res.Unmet)
	}
}
