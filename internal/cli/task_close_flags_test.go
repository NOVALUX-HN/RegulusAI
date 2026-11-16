package cli

import "testing"

// NOV-78: traceability skips are explicit and recorded, never silent.
// validateCloseSkip is pure flag validation (no git I/O) so the rule
// is pinned without fixtures.
func TestValidateCloseSkip(t *testing.T) {
	cases := []struct {
		name      string
		issue     string
		skip      bool
		justified string
		wantErr   bool
	}{
		{name: "no skip allows empty flags", issue: "", skip: false, justified: "", wantErr: false},
		{name: "no skip allows issue without justification", issue: "NOV-64", skip: false, justified: "", wantErr: false},
		{name: "skip with issue and reason allows", issue: "NOV-64", skip: true, justified: "hotfix post-fix", wantErr: false},
		{name: "skip without issue refuses", issue: "", skip: true, justified: "reason", wantErr: true},
		{name: "skip without reason refuses", issue: "NOV-64", skip: true, justified: "  ", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateCloseSkip(tc.issue, tc.skip, tc.justified)
			if (got != "") != tc.wantErr {
				t.Fatalf("validateCloseSkip(%q,%v,%q) = %q, wantErr=%v", tc.issue, tc.skip, tc.justified, got, tc.wantErr)
			}
		})
	}
}
