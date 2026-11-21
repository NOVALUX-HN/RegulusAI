// RenderTierPlan tests (NOV-119 CRAP paydown): table-driven coverage of
// the human selection report over minimal TierEvidenceManifest +
// TierSelection fixtures: included/omitted rows, holdout sealed vs
// included, capability and changed lines, short/long/empty SHAs and
// unknown-tier budgets. Pure: no filesystem, no shell.
package bench

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTierPlanTable(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	corpus := []AgentTask{
		{ID: "AT1", Lang: "go", BudgetS: 60},
		{ID: "AT2", Lang: "node", BudgetS: 30},
	}
	splits := AgentSplits{Version: "v1", Seed: 7, Exploration: []string{"AT1"}, Holdout: []string{"AT2"}}
	cases := []struct {
		name     string
		manifest TierEvidenceManifest
		sel      TierSelection
		want     []string
		absent   []string
	}{
		{
			name: "L2 focused with capability changed and sealed holdout",
			manifest: TierEvidenceManifest{TierVersion: TierVersion, Tier: TierL2,
				GitHead: "abc123", StartedAt: "2026-11-19T12:00:00Z",
				PlanFile: TierPlanFile, PlanSHA: "abcdef1234567890deadbeef"},
			sel: TierSelection{Tier: TierL2, SuiteVersion: "s",
				SplitsVersion: "v1", SplitsSeed: 7, CorpusSHA: "corpussha1234567890",
				Changed:    []string{"internal/a/a.go"},
				Capability: "go", Included: []string{"AT1"},
				Omitted: []TierOmission{{ID: "AT2", Split: "holdout", Reason: "holdout sealed below L3"}},
				Reason:  "focused"},
			want: []string{
				"tier L2", "capability filter: go", "changed: internal/a/a.go",
				"executed 1:", "- AT1 [exploration]", "L2 regression samples AT1",
				"omitted 1:", "- AT2 [holdout]", "holdout sealed below L3",
				"holdout: sealed", "feedback ceilings",
				"run: regulus bench agent-tasks --task AT1 --split exploration",
				"budgets: run <= 1800s, agent <= 120s",
				"evidence: tier-plan.json sha256:abcdef123456 git abc123",
				"corpus corpussha123",
			},
			absent: []string{"holdout: included"},
		},
		{
			name: "L3 full includes holdout without capability or changed",
			manifest: TierEvidenceManifest{TierVersion: TierVersion, Tier: TierL3,
				GitHead: "", StartedAt: "2026-11-19T12:00:00Z",
				PlanFile: TierPlanFile, PlanSHA: ""},
			sel: TierSelection{Tier: TierL3, SuiteVersion: "s",
				SplitsVersion: "v1", SplitsSeed: 7, CorpusSHA: "",
				Included: []string{"AT1", "AT2"}, Reason: "full"},
			want: []string{
				"tier L3", "executed 2:", "- AT1 [exploration]", "- AT2 [holdout]",
				"omitted 0:", "holdout: included",
				"L3 full corpus includes AT1", "L3 full corpus includes AT2",
				"run: regulus bench agent-tasks --task AT1,AT2 --split all",
				"budgets: run <= 7200s, agent <= 120s",
				"sha256:unknown git unknown", "corpus unknown",
			},
			absent: []string{"capability filter:", "changed:", "holdout: sealed"},
		},
		{
			name: "unknown tier seals holdout and zeroes run budget",
			manifest: TierEvidenceManifest{TierVersion: TierVersion, Tier: "LX",
				GitHead: "deadbeef", StartedAt: "2026-11-19T12:00:00Z",
				PlanFile: TierPlanFile, PlanSHA: "abc"},
			sel: TierSelection{Tier: "LX", SuiteVersion: "s",
				SplitsVersion: "v1", SplitsSeed: 7, CorpusSHA: "xy",
				Included: []string{"AT1"}, Reason: "unknown"},
			want: []string{
				"tier LX", "executed 1:", "holdout: sealed",
				"L3 full corpus includes AT1", "budgets: run <= 0s, agent <= 120s",
				"sha256:abc git deadbeef", "corpus xy",
			},
			absent: []string{"capability filter:", "changed:", "holdout: included"},
		},
		{
			name: "L0 smoke minimal with short SHAs",
			manifest: TierEvidenceManifest{TierVersion: TierVersion, Tier: TierL0,
				GitHead: "  ", StartedAt: "2026-11-19T12:00:00Z",
				PlanFile: TierPlanFile, PlanSHA: "abc"},
			sel: TierSelection{Tier: TierL0, SuiteVersion: "s",
				SplitsVersion: "v1", SplitsSeed: 7, CorpusSHA: "xy",
				Included: []string{"AT1"}, Reason: "smoke"},
			want: []string{
				"tier L0", "executed 1:", "- AT1 [exploration]",
				"L0 smoke covers go (AT1)", "omitted 0:", "holdout: sealed",
				"budgets: run <= 300s, agent <= 120s",
				"sha256:abc git unknown",
			},
			absent: []string{"capability filter:", "changed:", "holdout: included"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderTierPlan(tc.manifest, tc.sel, corpus, splits, now)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("report lacks %q:\n%s", w, got)
				}
			}
			for _, w := range tc.absent {
				if strings.Contains(got, w) {
					t.Errorf("report must not contain %q:\n%s", w, got)
				}
			}
		})
	}
}
