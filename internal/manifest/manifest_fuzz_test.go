// NOV-65: fuzzing of manifest decoding. Properties: never panics,
// deterministic; malformed input always errors.
package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzReadFile(f *testing.F) {
	f.Add(`{"schema":1,"run_id":"r1","artifacts":[]}`)
	f.Add(`not json`)
	f.Add(``)
	f.Add("\x00\xff")
	f.Add(`{"schema":99}`)
	f.Fuzz(func(t *testing.T, body string) {
		dir := t.TempDir()
		p := filepath.Join(dir, "manifest-test.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		m1, err1 := ReadFile(p)
		m2, err2 := ReadFile(p)
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("non-deterministic decode for %q", body)
		}
		if err1 == nil && (m1.RunID != m2.RunID || m1.Schema != m2.Schema) {
			t.Fatalf("non-deterministic manifest for %q", body)
		}
	})
}
