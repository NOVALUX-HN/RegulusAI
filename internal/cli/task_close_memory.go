package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/sextant"
)

// M3 auto-save on close: emitting CLOSED also records one session
// observation with the summary (id, spec, receipts) — the Engram-style
// habit, without ceremony. Best-effort and idempotent, and never part of
// the close verdict: a repeat close finds the topic chain non-empty and
// skips, CheckDuplicate (DedupThreshold) refuses saves duplicating pinned
// or lesson-equivalent knowledge, and any other failure warns on stderr
// while the CLOSED verdict stands.

// closeNoteTopic groups every auto-saved close note of one unit, so the
// second close of the same unit is a dedup hit, not a second note.
func closeNoteTopic(chart, id string) string {
	return "task-close/" + chart + "/" + id
}

// chartDomainRe reads the chart's `Domain:` line (the lesson-domain line
// the scaffold and the plugin hooks use).
var chartDomainRe = regexp.MustCompile(`(?m)^Domain:\s*(.+?)\s*$`)

// chartDomain returns the chart's declared domain, or "" when the chart
// declares none (absent line or the `<name>` scaffold placeholder) — a
// close note never invents a domain.
func chartDomain(doc string) string {
	m := chartDomainRe.FindStringSubmatch(doc)
	if len(m) != 2 {
		return ""
	}
	if d := strings.TrimSpace(m[1]); d != "" && d != "<name>" {
		return d
	}
	return ""
}

// closeTaskSpec finds the Spec cell of one task row ("" when unknown).
func closeTaskSpec(doc, id string) string {
	tasks, _ := sextant.ParseTasks(doc)
	for _, t := range tasks {
		if t.ID == id {
			return t.Spec
		}
	}
	return ""
}

// buildCloseNote renders the auto-saved observation for a closed unit.
func buildCloseNote(chart, id, doc string, res sextant.CloseResult, gitHead string) *almanac.Observation {
	var b strings.Builder
	fmt.Fprintf(&b, "CLOSED %s/%s (%s): spec OK + %d receipt(s) PASS.\n", chart, id, res.Class, len(res.Receipts))
	if spec := closeTaskSpec(doc, id); spec != "" {
		fmt.Fprintf(&b, "spec: %s\n", spec)
	}
	for _, r := range res.Receipts {
		fmt.Fprintf(&b, "receipt: %s\n", r)
	}
	return &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "task close " + chart + "/" + id,
		Content: strings.TrimRight(b.String(), "\n"), TopicKey: closeNoteTopic(chart, id), Domain: chartDomain(doc),
		Provenance: &almanac.Provenance{Task: chart + "/" + id, GitHead: gitHead, Receipts: res.Receipts},
	}
}

// autoSaveCloseNote records the close observation best-effort. It never
// fails the close: every path either saves, skips as a duplicate, or
// warns on stderr.
func (c CLI) autoSaveCloseNote(chart, id, doc string, res sextant.CloseResult) {
	topic := closeNoteTopic(chart, id)
	if hist, err := almanac.History(c.dir(), topic); err == nil && len(hist) != 0 {
		fmt.Fprintf(c.Stdout, "  memory: close observation already recorded (topic %s), skipping duplicate\n", topic)
		return
	}
	obs := buildCloseNote(chart, id, doc, res, gitHeadBounded(c.runner()))
	if err := almanac.CheckDuplicate(c.dir(), obs); err != nil {
		var dup *almanac.DuplicateError
		if errors.As(err, &dup) {
			fmt.Fprintf(c.Stdout, "  memory: close observation duplicates %s, skipping\n", dup.ID)
			return
		}
		fmt.Fprintf(c.Stderr, "warning: close observation not checked (%v) — the CLOSED verdict stands\n", err)
		return
	}
	saved, err := almanac.Save(c.dir(), obs, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(c.Stderr, "warning: close observation not saved (%v) — the CLOSED verdict stands\n", err)
		return
	}
	c.refreshIndexBestEffort("task-close")
	fmt.Fprintf(c.Stdout, "  memory: close observation saved %s\n", saved.ID)
}
