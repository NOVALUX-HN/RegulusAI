// Table-driven PlanParallelLevels cases (pure, no disk): collision
// order, explicit-order wins and cycles. Complements the behavioral
// tests in parallel_test.go.
package orchestrate

import (
	"strings"
	"testing"
)

func planTask(id string, files []string, deps ...string) ParallelTask {
	return ParallelTask{ID: id, Title: "Task " + id, Files: files, DependsOn: deps}
}

func equalLevels(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

func equalPairs(a, b [][2]string) bool {
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

func TestPlanParallelLevelsTable(t *testing.T) {
	tests := []struct {
		name       string
		tasks      []ParallelTask
		wantLevels [][]string
		wantSerial [][2]string
		wantErr    string
	}{
		{
			name:       "independent share one sorted level",
			tasks:      []ParallelTask{planTask("b", []string{"b.txt"}), planTask("a", []string{"a.txt"})},
			wantLevels: [][]string{{"a", "b"}},
		},
		{
			name:       "colliding pair serializes in sorted id order",
			tasks:      []ParallelTask{planTask("b", []string{"shared.txt"}), planTask("a", []string{"shared.txt"})},
			wantLevels: [][]string{{"a"}, {"b"}},
			wantSerial: [][2]string{{"a", "b"}},
		},
		{
			name:       "container collision serializes",
			tasks:      []ParallelTask{planTask("a", []string{"pkg/foo"}), planTask("b", []string{"pkg/foo/bar.go"})},
			wantLevels: [][]string{{"a"}, {"b"}},
			wantSerial: [][2]string{{"a", "b"}},
		},
		{
			name: "explicit order wins over collision",
			tasks: []ParallelTask{
				planTask("b", []string{"shared.txt"}, "a"),
				planTask("a", []string{"shared.txt"}),
			},
			wantLevels: [][]string{{"a"}, {"b"}},
		},
		{
			name: "chain orders one per level",
			tasks: []ParallelTask{
				planTask("c", nil, "b"),
				planTask("b", nil, "a"),
				planTask("a", nil),
			},
			wantLevels: [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name: "diamond joins",
			tasks: []ParallelTask{
				planTask("d", nil, "b", "c"),
				planTask("c", nil, "a"),
				planTask("b", nil, "a"),
				planTask("a", nil),
			},
			wantLevels: [][]string{{"a"}, {"b", "c"}, {"d"}},
		},
		{
			name: "transitive collision skips redundant edge",
			tasks: []ParallelTask{
				planTask("c", []string{"shared.txt"}, "b"),
				planTask("b", []string{"shared.txt"}, "a"),
				planTask("a", []string{"shared.txt"}),
			},
			wantLevels: [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name: "cycle fails naming the loop",
			tasks: []ParallelTask{
				planTask("a", nil, "b"),
				planTask("b", nil, "a"),
			},
			wantErr: "cycle",
		},
		{
			name: "self cycle fails",
			tasks: []ParallelTask{
				planTask("a", nil, "a"),
			},
			wantErr: "must not depend on itself",
		},
		{
			name: "unknown dependency fails validation",
			tasks: []ParallelTask{
				planTask("a", nil, "ghost"),
			},
			wantErr: "unknown task",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			levels, serialized, err := PlanParallelLevels(tc.tasks)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("PlanParallelLevels err = %v, want containing %q", err, tc.wantErr)
				}
				if levels != nil || serialized != nil {
					t.Fatalf("failed plan must return nil levels/serialized, got %v/%v", levels, serialized)
				}
				return
			}
			if err != nil {
				t.Fatalf("PlanParallelLevels: %v", err)
			}
			if !equalLevels(levels, tc.wantLevels) {
				t.Fatalf("levels = %v, want %v", levels, tc.wantLevels)
			}
			if !equalPairs(serialized, tc.wantSerial) {
				t.Fatalf("serialized = %v, want %v", serialized, tc.wantSerial)
			}
			// Every task runs exactly once, in merge order.
			flat := FlattenParallelLevels(levels)
			if len(flat) != len(tc.tasks) {
				t.Fatalf("flat = %v, want %d tasks", flat, len(tc.tasks))
			}
			seen := map[string]bool{}
			for _, id := range flat {
				if seen[id] {
					t.Fatalf("task %q appears twice in %v", id, flat)
				}
				seen[id] = true
			}
		})
	}
}
