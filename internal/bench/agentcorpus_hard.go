// NOV-77 hard tasks: discriminating tasks for a strong baseline.
// Plain-text fixtures stay in backticks: no escapes, no surprises.
// Each one fails pre-fix (verified by TestAgentCorpusV2AllFailPreFix)
// and passes with the straightforward correct fix.
package bench

// hardCorpusTasks1 returns hard tasks AT37-AT40: stable sort,
// multiline CSV, continuation-line logs and NUL-safe duplicates.
func hardCorpusTasks1() []AgentTask {
	return []AgentTask{
		{
			ID: "AT37", Title: "stable sort by score desc", Lang: "go",
			Prompt: "Sort rows by Score descending, keeping input order for ties.",
			Files: map[string]string{
				"stable.go": `package m

import "sort"

// Row is one scored entry.
type Row struct {
	Name  string
	Score int
}

// ByScoreDesc sorts rows by score descending, ties keep input order.
func ByScoreDesc(rs []Row) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].Score > rs[j].Score })
}
`,
				"stable_test.go": `package m

import (
	"reflect"
	"testing"
)

func TestByScoreDescTies(t *testing.T) {
	rs := []Row{{"n1", 1}, {"n2", 5}, {"n3", 3}, {"n4", 5}, {"n5", 2}, {"n6", 4}, {"n7", 5}, {"n8", 0}, {"n9", 3}, {"n10", 5}, {"n11", 1}, {"n12", 4}, {"n13", 2}, {"n14", 5}, {"n15", 0}, {"n16", 3}, {"n17", 1}, {"n18", 5}, {"n19", 2}, {"n20", 4}}
	ByScoreDesc(rs)
	var names []string
	for _, r := range rs {
		names = append(names, r.Name)
	}
	want := []string{"n2", "n4", "n7", "n10", "n14", "n18", "n6", "n12", "n20", "n3", "n9", "n16", "n5", "n13", "n19", "n1", "n11", "n17", "n8", "n15"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("order = %v, want %v", names, want)
	}
}
`,
			},
			Check:   []string{"go", "test", "./..."},
			BudgetS: 120,
		},
		{
			ID: "AT38", Title: "CSV with multiline quoted fields", Lang: "node",
			Prompt: "Parse CSV rows; quoted fields may span lines and hold commas.",
			Files: map[string]string{
				"csv.js": `export function parse(text) {
  return text.trim().split("\n").map((l) => l.split(","));
}
`,
				"data.csv": "a,\"b\nc\",d\ne,f\n",
				"test.mjs": `import assert from 'node:assert';
import { readFileSync } from 'node:fs';
import { parse } from './csv.js';
const text = readFileSync(new URL('./data.csv', import.meta.url), 'utf8');
assert.deepStrictEqual(parse(text), [['a', 'b\nc', 'd'], ['e', 'f']]);
console.log('ok');
`,
			},
			Check:   []string{"node", "test.mjs"},
			BudgetS: 60,
		},
		{
			ID: "AT39", Title: "log continuation lines", Lang: "python",
			Prompt: "Count ERROR logical lines: a trailing backslash joins the next line.",
			Files: map[string]string{
				"app.log": `INFO start
ERROR disk \
ERROR is not fatal, continuing
WARN low mem
ERROR oom killer
`,
				"count.py": `def count_errors(path):
    n = 0
    with open(path) as f:
        for line in f:
            if line.startswith("ERROR"):
                n += 1
    return n


if __name__ == "__main__":
    print(count_errors("app.log"))
`,
				"test_count.py": `import subprocess
out = subprocess.run(["python3", "count.py"], capture_output=True, text=True)
if out.returncode != 0:
    out = subprocess.run(["python", "count.py"], capture_output=True, text=True)
assert out.stdout.strip() == "2", out.stdout
print("ok")
`,
			},
			Check:   []string{"bash", "-c", "python3 test_count.py || python test_count.py"},
			BudgetS: 120,
		},
		{
			ID: "AT40", Title: "duplicate files by content, NUL-safe", Lang: "shell",
			Prompt: "Print duplicate .txt files by content (one per line, basenames only).",
			Files: map[string]string{
				"a.txt":      "x\n",
				"b.txt":      "x\n",
				"sp ace.txt": "x\n",
				"c.txt":      "y\n",
				"dup.sh":     "#!/usr/bin/env bash\nfor f in $(ls *.txt); do\n  md5sum \"$f\"\ndone | sort -k1,1 | uniq -w 32 -D | awk '{print $2}'\n",
			},
			Check:   []string{"bash", "-c", "diff <(bash dup.sh | LC_ALL=C sort) <(printf 'b.txt\\nsp ace.txt\\n')"},
			BudgetS: 60,
		},
	}
}

// hardCorpusTasks2 returns hard tasks AT41-AT42: bounded parallel map
// with cancel and the quoted-key JSON path getter.
func hardCorpusTasks2() []AgentTask {
	return []AgentTask{
		{
			ID: "AT41", Title: "bounded parallel map with cancel", Lang: "go",
			Prompt: "Run at most limit tasks at once, keep input order, first error wins, honor cancel.",
			Files: map[string]string{
				"pool.go": `package m

import "context"

// Map applies fn to each input with at most limit in flight, preserving
// input order. It returns the first error and stops launching new work
// when ctx is done.
func Map[T any, R any](ctx context.Context, limit int, xs []T, fn func(context.Context, T) (R, error)) ([]R, error) {
	out := make([]R, len(xs))
	for i, x := range xs {
		r, err := fn(ctx, x)
		if err != nil {
			return nil, err
		}
		out[i] = r
	}
	return out, nil
}
`,
				"pool_test.go": `package m

import (
	"context"
	"testing"
	"time"
)

func TestMapParallel(t *testing.T) {
	started := make(chan int, 4)
	release := make(chan struct{})
	fn := func(ctx context.Context, x int) (int, error) {
		started <- x
		<-release
		return x * 2, nil
	}
	done := make(chan struct{})
	var got []int
	var err error
	go func() {
		defer close(done)
		got, err = Map(context.Background(), 2, []int{1, 2, 3, 4}, fn)
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("second task never started: no parallelism")
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("second task never started: no parallelism")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Map did not finish after release")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0] != 2 || got[1] != 4 || got[2] != 6 || got[3] != 8 {
		t.Fatalf("got = %v, want [2 4 6 8]", got)
	}
}

func TestMapCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Map(ctx, 2, []int{1, 2, 3}, func(ctx context.Context, x int) (int, error) { return x, nil }); err == nil {
		t.Fatal("want error for pre-canceled context")
	}
}
`,
			},
			Check:   []string{"go", "test", "./...", "-count=1"},
			BudgetS: 180,
		},
		{
			ID: "AT42", Title: "JSON path getter", Lang: "node",
			Prompt: "Implement get(obj, path) with dotted keys, [n] indexes and quoted keys.",
			Files: map[string]string{
				"jpath.js": `export function get(obj, path) {
  return undefined;
}
`,
				"test.mjs": `import assert from 'node:assert';
import { get } from './jpath.js';
assert.strictEqual(get({a: [{b: 3}]}, 'a[0].b'), 3);
assert.strictEqual(get({a: {'b.c': 2}}, 'a["b.c"]'), 2);
assert.strictEqual(get({x: 1}, 'y'), undefined);
assert.strictEqual(get({x: 1}, 'x.y'), undefined);
console.log('ok');
`,
			},
			Check:   []string{"node", "test.mjs"},
			BudgetS: 60,
		},
	}
}

// hardCorpusTasks3 returns hard tasks AT43-AT44: the TTL cache with
// injected clock and INI interpolation.
func hardCorpusTasks3() []AgentTask {
	return []AgentTask{
		{
			ID: "AT43", Title: "TTL cache with injected clock (holdout)", Lang: "go",
			Prompt: "Implement an LRU cache with absolute TTL from an injected clock.",
			Files: map[string]string{
				"ttl.go": `package m

// Cache is a capacity-bound store with absolute TTL (no sliding).
// now supplies unix seconds; tests inject a fake clock.
type Cache struct {
	capacity int
	ttl      int64
	now      func() int64
	items    map[string]entry
	order    []string
}

type entry struct {
	v   int
	exp int64
}

// NewLRU builds a cache holding up to capacity entries, each living
// ttl seconds from Set.
func NewLRU(capacity int, ttl int64, now func() int64) *Cache {
	return &Cache{capacity: capacity, ttl: ttl, now: now, items: map[string]entry{}}
}

// Set stores v, evicting the oldest when over capacity.
func (c *Cache) Set(k string, v int) {
	c.items[k] = entry{v: v, exp: 1}
	c.order = append(c.order, k)
}

// Get returns the value for k, or false when absent or expired.
func (c *Cache) Get(k string) (int, bool) {
	e, ok := c.items[k]
	if !ok {
		return 0, false
	}
	return e.v, true
}
`,
				"ttl_test.go": `package m

import "testing"

func TestTTLExpires(t *testing.T) {
	now := int64(1000)
	c := NewLRU(10, 10, func() int64 { return now })
	c.Set("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("get = %v, %v; want 1 true", v, ok)
	}
	now = 1011
	if _, ok := c.Get("a"); ok {
		t.Fatal("expired entry must miss")
	}
}

func TestTTLEvictsOldest(t *testing.T) {
	now := int64(0)
	c := NewLRU(2, 1000, func() int64 { return now })
	c.Set("a", 1)
	c.Set("b", 2)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a must hit before eviction")
	}
	c.Set("c", 3)
	if _, ok := c.Get("b"); ok {
		t.Fatal("b must be evicted as oldest untouched")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("a must survive, got %v %v", v, ok)
	}
}
`,
			},
			Check:   []string{"go", "test", "./..."},
			BudgetS: 120,
		},
		{
			ID: "AT44", Title: "INI with interpolation (holdout)", Lang: "python",
			Files: map[string]string{
				"app.conf": `[db]
host = localhost
port = 5432
[app]
name = myapp
url = postgres://${db.host}:${db.port}/main
local = ${name}-local
`,
				"ini.py":      "def load(path):\n    return {}\n",
				"test_ini.py": "from ini import load\ncfg = load(\"app.conf\")\nassert cfg[\"app\"][\"url\"] == \"postgres://localhost:5432/main\", cfg\nassert cfg[\"app\"][\"local\"] == \"myapp-local\", cfg\nassert cfg[\"db\"][\"port\"] == \"5432\", cfg\ntry:\n    load(\"app.conf\")\nexcept Exception:\n    raise AssertionError(\"load must not raise on valid file\")\nprint(\"ok\")\n",
			},
			Prompt:  "Implement INI load with sections, ${section.key}/${key} interpolation, duplicate = error, missing ref = error.",
			Check:   []string{"bash", "-c", "python3 test_ini.py || python test_ini.py"},
			BudgetS: 120,
		},
	}
}

// hardCorpusTasks returns the v3 hard slice: multi-step reasoning,
// unstable-sort awareness, stateful parsing, NUL-safe shell,
// order-preserving parallelism with cancel, quoted-key paths,
// TTL caches with injected clocks, and INI interpolation. The three
// helpers hold consecutive ID ranges so the order stays byte-identical.
func hardCorpusTasks() []AgentTask {
	out := append([]AgentTask{}, hardCorpusTasks1()...)
	out = append(out, hardCorpusTasks2()...)
	out = append(out, hardCorpusTasks3()...)
	return out
}

// hardCorpusNote documents why these tasks exist separately: they were
// added to discriminate a saturated baseline (NOV-77) after v2 hit 100%
// pass on the reference model. They join the same splits validation,
// anti-leakage rules and evidence pipeline as the base corpus.
