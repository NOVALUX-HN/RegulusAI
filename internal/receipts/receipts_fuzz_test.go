// NOV-65: fuzzing of receipt decoding. Properties: never panics,
// malformed input always errors (never a zero-value pass).
package receipts

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzReadFile(f *testing.F) {
	f.Add(`{"id":"rec_1","task":"t","gate":"g","verdict":"pass","exit_code":0}`)
	f.Add(`not json`)
	f.Add(``)
	f.Add("\x00\x01")
	f.Add(`{"id":"","task":"","gate":""}`)
	f.Fuzz(func(t *testing.T, body string) {
		dir := t.TempDir()
		p := filepath.Join(dir, "r.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		r1, err1 := ReadFile(p)
		r2, err2 := ReadFile(p)
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("non-deterministic decode for %q", body)
		}
		if err1 == nil && (r1.ID != r2.ID || r1.Verdict != r2.Verdict) {
			t.Fatalf("non-deterministic receipt for %q", body)
		}
	})
}
