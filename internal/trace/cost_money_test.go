// CostMoneyByTaskFiltered scope tests: fixed tariffs over t.TempDir,
// covering task/agent/run filters, the unattributed bucket and the
// addCounts summation (including the hostile-total failure).
package trace

import (
	"math"
	"strings"
	"testing"
)

func fixedMoneyRates(t *testing.T) []RateCard {
	t.Helper()
	r, err := ParseRateCard([]byte(f115RateCard))
	if err != nil {
		t.Fatalf("fixed tariff: %v", err)
	}
	return []RateCard{r}
}

func TestCostMoneyByTaskFilteredScopes(t *testing.T) {
	dir := t.TempDir()
	rates := fixedMoneyRates(t)
	recordEntry(t, dir, "s1", "T1", Entry{
		Agent: "worker-a", ExecutionID: "r1",
		InputTokens: 1000, OutputTokens: 100,
		Model: "beam-free", Host: "opencode", HostVersion: "2.2.5",
		UsageSource: SourceMeasuredHost,
	})
	recordEntry(t, dir, "s1", "T1", Entry{
		Agent: "worker-a", ExecutionID: "r1",
		InputTokens: 500, OutputTokens: 50,
		Model: "beam-free", Host: "opencode", HostVersion: "2.2.5",
		UsageSource: SourceMeasuredHost,
	})
	recordEntry(t, dir, "s1", "T2", Entry{
		Agent: "worker-b", ExecutionID: "r2",
		InputTokens: 200, OutputTokens: 20,
		Model: "beam-free", Host: "opencode", HostVersion: "2.2.5",
		UsageSource: SourceMeasuredHost,
	})
	recordEntry(t, dir, "s2", "", Entry{DurationMS: 5})

	cases := []struct {
		name      string
		task      string
		agent     string
		run       string
		wantTasks []string
	}{
		{name: "sin filtros trae todo", wantTasks: []string{"T1", "T2", UnattributedTask}},
		{name: "filtro task", task: "T1", wantTasks: []string{"T1"}},
		{name: "filtro agent", agent: "worker-b", wantTasks: []string{"T2"}},
		{name: "filtro run", run: "r2", wantTasks: []string{"T2"}},
		{name: "filtro run ausente vacio", run: "nope", wantTasks: nil},
		{name: "filtro unattributed", task: UnattributedTask, wantTasks: []string{UnattributedTask}},
		{name: "task y agent combinados", task: "T1", agent: "worker-a", wantTasks: []string{"T1"}},
		{name: "task y agent sin cruce vacio", task: "T1", agent: "worker-b", wantTasks: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CostMoneyByTaskFiltered(dir, rates, tc.task, tc.agent, tc.run)
			if err != nil {
				t.Fatalf("CostMoneyByTaskFiltered: %v", err)
			}
			seen := map[string]bool{}
			for _, m := range got {
				seen[m.Task] = true
			}
			if len(seen) != len(tc.wantTasks) {
				t.Fatalf("tasks = %v, want %v (full %+v)", seen, tc.wantTasks, got)
			}
			for _, w := range tc.wantTasks {
				if !seen[w] {
					t.Fatalf("falta bucket %q en %v", w, seen)
				}
			}
			if tc.task == "T1" && tc.agent == "" && tc.run == "" {
				for _, m := range got {
					if m.Task != "T1" {
						continue
					}
					if m.InputTokens != 1500 || m.OutputTokens != 150 {
						t.Fatalf("addCounts suma = %d+%d, want 1500+150 (%+v)", m.InputTokens, m.OutputTokens, m)
					}
					if !m.Known || m.Total <= 0 {
						t.Fatalf("T1 con tarifa explicita debe priced: %+v", m)
					}
				}
			}
			if tc.task == UnattributedTask {
				for _, m := range got {
					if m.Known {
						t.Fatalf("unattributed sin tokens debe unknown, nunca cero: %+v", m)
					}
				}
			}
		})
	}
}

func TestMoneyAddCountsHostileFails(t *testing.T) {
	m := &TaskMoney{Task: "T1", InputTokens: math.MaxInt64}
	e := Entry{InputTokens: 1}
	if err := moneyAddCounts(m, e, "k"); err == nil || !strings.Contains(err.Error(), "money fold") {
		t.Fatalf("total hostil debe fallar con bucket, got %v", err)
	}
	if !moneyFilterKeeps(Entry{Task: "T1", Agent: "a", ExecutionID: "r"}, "T1", "a", "r") {
		t.Fatal("filtros exactos deben pasar")
	}
	if moneyFilterKeeps(Entry{Task: "T1"}, "T2", "", "") {
		t.Fatal("task distinta debe filtrarse")
	}
}
