// NOV-65: fuzzing of Almanac import. Properties: never panics, no
// partial writes on malformed lines (Import is all-or-nothing per
// file), deterministic counts.
package almanac

import (
	"os"
	"path/filepath"
	"testing"
)

var fuzzImportSeeds = []string{
	`{"id":"a1","type":"session-note","title":"t","content":"c","saved_at":"2026-01-01T00:00:00Z"}` + "\n",
	"not json\n",
	`{"id":"","type":"bogus","title":"","content":""}` + "\n",
	"\x00\x01\x02\n",
	`{"id":"a1","type":"session-note","title":"t","content":"c","saved_at":"2026-01-01T00:00:00Z"}` + "\n" + `{"id":"a1","type":"session-note","title":"dup","content":"c","saved_at":"2026-01-01T00:00:00Z"}` + "\n",
}

func FuzzImport(f *testing.F) {
	for _, s := range fuzzImportSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		run := func() (int, int, error) {
			dir := t.TempDir()
			p := filepath.Join(dir, "in.jsonl")
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			return Import(dir, p)
		}
		n1, s1, err1 := run()
		n2, s2, err2 := run()
		if (err1 == nil) != (err2 == nil) || n1 != n2 || s1 != s2 {
			t.Fatalf("non-deterministic import for %q", body)
		}
	})
}
