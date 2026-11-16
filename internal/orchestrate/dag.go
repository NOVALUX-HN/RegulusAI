// Parallel task DAG (F12.2/NOV-97): decompose work into tasks, detect
// dependencies and file collisions, and serialize when they share
// state.
//
// A ParallelTask names the repo-relative files it touches. Two tasks
// collide when a normalized file is equal or when one path contains
// the other (a task touching "pkg/foo" collides with one touching
// "pkg/foo/bar.go"): sharing state means sharing order, so the plan
// serializes them. Explicit DependsOn edges order work the DAG names;
// implicit collision edges order the rest (sorted id order, explicit
// order wins when it already orders a colliding pair). The result is
// deterministic levels: every task runs exactly once, dependencies
// first, colliding tasks never in the same level.
package orchestrate

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// ParallelVersion pins the parallel-plan document shape.
const ParallelVersion = "parallel-v1"

// MaxParallelTasks bounds one parallel run: the scheduler, the merge
// order and the report all stay readable and the exec log stays
// bounded.
const MaxParallelTasks = 64

// ParallelTask is one unit of parallel work: an id, a human title,
// the repo-relative files it touches (collision domain), explicit
// dependency edges, the shell command the worker runs inside its
// worktree (empty means plan/record only: the branch merges trivially),
// the F12.1 routing hint and the restricted scope request (F11.8: the
// worker keeps only the intersection with its parent), plus optional
// wall/token estimates used for the dry-run ceilings.
type ParallelTask struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Files          []string `json:"files"`
	DependsOn      []string `json:"depends_on,omitempty"`
	Command        string   `json:"command,omitempty"`
	Agent          string   `json:"agent,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
	EstimateMS     int64    `json:"estimate_ms,omitempty"`
	EstimateTokens int64    `json:"estimate_tokens,omitempty"`
}

// ValidateParallelTasks refuses hostile or ambiguous plans before
// anything runs: task count, id charset (same conservative rule as
// gate task ids), single-line titles, repo-relative files (no
// absolute, no "..", no NUL/newline), known dependency targets
// without self-edges or duplicates, bounded commands/scopes and
// non-negative estimates.
func ValidateParallelTasks(tasks []ParallelTask) error {
	if len(tasks) == 0 {
		return fmt.Errorf("orchestrate parallel: at least one task is required")
	}
	if len(tasks) > MaxParallelTasks {
		return fmt.Errorf("orchestrate parallel: at most %d tasks, got %d", MaxParallelTasks, len(tasks))
	}
	ids := map[string]bool{}
	for i := range tasks {
		t := &tasks[i]
		if !validParallelID(t.ID) {
			return fmt.Errorf("orchestrate parallel: invalid task id %q (letters, digits, '.', '_', '-', '/'; no '..')", t.ID)
		}
		if ids[t.ID] {
			return fmt.Errorf("orchestrate parallel: duplicate task id %q", t.ID)
		}
		ids[t.ID] = true
		if strings.TrimSpace(t.Title) == "" {
			return fmt.Errorf("orchestrate parallel: task %q needs a title", t.ID)
		}
		if len(t.Title) > 256 || strings.ContainsAny(t.Title, "\n\r") {
			return fmt.Errorf("orchestrate parallel: task %q title must be a single line under 256 chars", t.ID)
		}
		if len(t.Files) > 32 {
			return fmt.Errorf("orchestrate parallel: task %q lists at most 32 files, got %d", t.ID, len(t.Files))
		}
		for _, f := range t.Files {
			if err := checkParallelFile(t.ID, f); err != nil {
				return err
			}
		}
		if len(t.DependsOn) > 32 {
			return fmt.Errorf("orchestrate parallel: task %q lists at most 32 dependencies, got %d", t.ID, len(t.DependsOn))
		}
		seenDep := map[string]bool{}
		for _, d := range t.DependsOn {
			if d == t.ID {
				return fmt.Errorf("orchestrate parallel: task %q must not depend on itself", t.ID)
			}
			if !validParallelID(d) {
				return fmt.Errorf("orchestrate parallel: task %q has invalid dependency %q", t.ID, d)
			}
			if seenDep[d] {
				return fmt.Errorf("orchestrate parallel: task %q duplicates dependency %q", t.ID, d)
			}
			seenDep[d] = true
		}
		if len(t.Command) > 4000 {
			return fmt.Errorf("orchestrate parallel: task %q command exceeds 4000 chars", t.ID)
		}
		if strings.IndexByte(t.Command, 0) >= 0 {
			return fmt.Errorf("orchestrate parallel: task %q command must not contain NUL", t.ID)
		}
		if len(t.Scopes) > 16 {
			return fmt.Errorf("orchestrate parallel: task %q requests at most 16 scopes, got %d", t.ID, len(t.Scopes))
		}
		for _, s := range t.Scopes {
			if strings.TrimSpace(s) == "" || len(s) > 64 || strings.ContainsAny(s, "\n\r") {
				return fmt.Errorf("orchestrate parallel: task %q has an invalid scope %q", t.ID, s)
			}
		}
		if len(t.Agent) > 64 || strings.ContainsAny(t.Agent, "\n\r") {
			return fmt.Errorf("orchestrate parallel: task %q agent must be a single line under 64 chars", t.ID)
		}
		if t.EstimateMS < 0 {
			return fmt.Errorf("orchestrate parallel: task %q estimate_ms must not be negative, got %d", t.ID, t.EstimateMS)
		}
		if t.EstimateTokens < 0 {
			return fmt.Errorf("orchestrate parallel: task %q estimate_tokens must not be negative, got %d", t.ID, t.EstimateTokens)
		}
	}
	for i := range tasks {
		for _, d := range tasks[i].DependsOn {
			if !ids[d] {
				return fmt.Errorf("orchestrate parallel: task %q depends on unknown task %q", tasks[i].ID, d)
			}
		}
	}
	return nil
}

// validParallelID mirrors the gate task-id rule: alphanumeric start,
// conservative charset, no traversal.
func validParallelID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	if strings.Contains(s, "..") {
		return false
	}
	for i, r := range s {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-' || r == '/'
		if !ok {
			return false
		}
		if i == 0 && r >= '0' && r <= '9' {
			// Gate ids allow a digit start via the same class; keep
			// the rule permissive here: digits may start an id.
		}
	}
	c := s[0]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
		return false
	}
	return true
}

// checkParallelFile refuses one touched file that is not a clean
// repo-relative path: empty, overlong, NUL/newline, absolute,
// drive-shaped or holding a ".." segment.
func checkParallelFile(taskID, f string) error {
	p := strings.TrimSpace(f)
	if p == "" {
		return fmt.Errorf("orchestrate parallel: task %q has an empty file entry", taskID)
	}
	if len(p) > 512 {
		return fmt.Errorf("orchestrate parallel: task %q file %q exceeds 512 chars", taskID, f)
	}
	if strings.IndexByte(p, 0) >= 0 || strings.ContainsAny(p, "\n\r") {
		return fmt.Errorf("orchestrate parallel: task %q file %q must be a single line", taskID, f)
	}
	if isAbsLike(p) {
		return fmt.Errorf("orchestrate parallel: task %q file %q must be repo-relative, not absolute", taskID, f)
	}
	slash := strings.ReplaceAll(p, "\\", "/")
	for _, seg := range strings.Split(slash, "/") {
		if seg == ".." {
			return fmt.Errorf("orchestrate parallel: task %q file %q must not contain '..'", taskID, f)
		}
	}
	return nil
}

// isAbsLike reports absolute-shaped values on either platform.
func isAbsLike(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "\\") {
		return true
	}
	return len(s) >= 2 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':'
}

// normalizeParallelFile folds one touched file to its collision key:
// slashes unified, cleaned, leading "./" dropped.
func normalizeParallelFile(f string) string {
	p := strings.TrimSpace(strings.ReplaceAll(f, "\\", "/"))
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "./")
	return p
}

// parallelFilesOf returns the normalized collision keys of one task,
// deduplicated and sorted.
func parallelFilesOf(t ParallelTask) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range t.Files {
		n := normalizeParallelFile(f)
		if n == "." || n == "" {
			continue
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// filesOverlap reports whether two normalized files share state:
// equal, or one contains the other on a slash boundary.
func filesOverlap(a, b string) bool {
	if a == b {
		return true
	}
	if strings.HasPrefix(b, a+"/") {
		return true
	}
	if strings.HasPrefix(a, b+"/") {
		return true
	}
	return false
}

// tasksCollide reports whether two tasks touch overlapping files.
func tasksCollide(a, b ParallelTask) bool {
	fa, fb := parallelFilesOf(a), parallelFilesOf(b)
	for _, x := range fa {
		for _, y := range fb {
			if filesOverlap(x, y) {
				return true
			}
		}
	}
	return false
}

// DetectParallelCollisions lists every colliding task pair (sorted id
// order, pairs sorted): the coordination the scheduler serializes.
func DetectParallelCollisions(tasks []ParallelTask) [][2]string {
	var pairs [][2]string
	for i := 0; i < len(tasks); i++ {
		for j := i + 1; j < len(tasks); j++ {
			a, b := tasks[i].ID, tasks[j].ID
			if !tasksCollide(tasks[i], tasks[j]) {
				continue
			}
			if a > b {
				a, b = b, a
			}
			pairs = append(pairs, [2]string{a, b})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	return pairs
}

// PlanParallelLevels orders tasks into deterministic execution levels:
// explicit DependsOn edges plus implicit collision edges (sorted id
// order; explicit order wins when it already orders a colliding pair).
// Every task appears exactly once; tasks in one level never depend on
// each other and never collide, so a level may run concurrently while
// levels run in order. Cycles fail closed naming the loop.
func PlanParallelLevels(tasks []ParallelTask) (levels [][]string, serialized [][2]string, err error) {
	if err := ValidateParallelTasks(tasks); err != nil {
		return nil, nil, err
	}
	byID := map[string]ParallelTask{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	// Explicit reachability: BFS per node over DependsOn edges
	// (dep -> task direction) so implicit edges never contradict an
	// explicit order.
	succ := map[string][]string{}
	for _, t := range tasks {
		for _, d := range t.DependsOn {
			succ[d] = append(succ[d], t.ID)
		}
	}
	reaches := func(from, to string) bool {
		if from == to {
			return true
		}
		seen := map[string]bool{from: true}
		queue := []string{from}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, nx := range succ[cur] {
				if nx == to {
					return true
				}
				if !seen[nx] {
					seen[nx] = true
					queue = append(queue, nx)
				}
			}
		}
		return false
	}
	// Edge set: explicit dep -> task plus implicit collision order.
	edges := map[string]map[string]bool{}
	addEdge := func(from, to string) {
		if from == to {
			return
		}
		if edges[from] == nil {
			edges[from] = map[string]bool{}
		}
		edges[from][to] = true
	}
	for _, t := range tasks {
		for _, d := range t.DependsOn {
			addEdge(d, t.ID)
		}
	}
	for _, p := range DetectParallelCollisions(tasks) {
		a, b := p[0], p[1]
		if reaches(a, b) || reaches(b, a) {
			continue
		}
		addEdge(a, b)
		serialized = append(serialized, p)
	}
	sort.Slice(serialized, func(i, j int) bool {
		if serialized[i][0] != serialized[j][0] {
			return serialized[i][0] < serialized[j][0]
		}
		return serialized[i][1] < serialized[j][1]
	})
	// Kahn in deterministic (sorted) order.
	indeg := map[string]int{}
	for _, t := range tasks {
		indeg[t.ID] = 0
	}
	for _, tos := range edges {
		for to := range tos {
			indeg[to]++
		}
	}
	var ready []string
	for id, n := range indeg {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	var order []string
	for len(ready) > 0 {
		cur := ready[0]
		ready = ready[1:]
		order = append(order, cur)
		var next []string
		for to := range edges[cur] {
			indeg[to]--
			if indeg[to] == 0 {
				next = append(next, to)
			}
		}
		ready = append(ready, next...)
		sort.Strings(ready)
	}
	if len(order) != len(tasks) {
		var loop []string
		for id, n := range indeg {
			if n > 0 {
				loop = append(loop, id)
			}
		}
		sort.Strings(loop)
		return nil, nil, fmt.Errorf("orchestrate parallel: dependency cycle involving %s", strings.Join(loop, ", "))
	}
	// Level assignment: level(task) = 1 + max(level(deps)); implicit
	// edges are deps for this purpose, so colliding tasks land on
	// strictly increasing levels.
	levelOf := map[string]int{}
	maxLevel := 0
	for _, id := range order {
		lv := 0
		// Predecessors: explicit deps plus any edge source.
		preds := map[string]bool{}
		for _, d := range byID[id].DependsOn {
			preds[d] = true
		}
		for from, tos := range edges {
			if tos[id] {
				preds[from] = true
			}
		}
		for p := range preds {
			if levelOf[p]+1 > lv {
				lv = levelOf[p] + 1
			}
		}
		levelOf[id] = lv
		if lv > maxLevel {
			maxLevel = lv
		}
	}
	levels = make([][]string, maxLevel+1)
	for _, id := range order {
		levels[levelOf[id]] = append(levels[levelOf[id]], id)
	}
	for i := range levels {
		sort.Strings(levels[i])
	}
	return levels, serialized, nil
}

// FlattenParallelLevels renders the merge order: levels concatenated,
// each level sorted — the deterministic integration sequence.
func FlattenParallelLevels(levels [][]string) []string {
	var out []string
	for _, lv := range levels {
		out = append(out, lv...)
	}
	return out
}
