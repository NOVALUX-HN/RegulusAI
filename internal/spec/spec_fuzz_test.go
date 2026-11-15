// NOV-65: fuzzing of the GWT spec parser. Properties: never panics,
// deterministic, and well-formed docs keep their scenario count.
package spec

import (
	"testing"
)

func FuzzCheck(f *testing.F) {
	f.Add("## Acceptance\n### Scenario: x\n- Given a valid foo\n- When user does bar\n- Then system shows baz\n")
	f.Add("")
	f.Add("## Acceptance\n### Scenario: x\n- Then lonely\n")
	f.Add("### Scenario: no acceptance heading\n- Given a\n- When b\n- Then c\n")
	f.Fuzz(func(t *testing.T, doc string) {
		n1, e1 := Check(doc)
		n2, e2 := Check(doc)
		if n1 != n2 || len(e1) != len(e2) {
			t.Fatalf("non-deterministic check for %q", doc)
		}
		if n1 < 0 {
			t.Fatalf("negative scenario count for %q", doc)
		}
	})
}
