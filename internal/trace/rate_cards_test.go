// Rate cards array cover (CRAP paydown sin cambiar comportamiento):
// 5 casos vacío/1 ok/oversize/trailing/índice error. Portable, sin
// disco ni red: solo documentos en memoria.
package trace

import (
	"strings"
	"testing"
)

func TestParseRateCardsTable(t *testing.T) {
	valid := "[" + f115RateCard + "]"
	cases := []struct {
		name    string
		doc     string
		wantLen int
		wantErr string
	}{
		{name: "empty array parses to no tariffs", doc: `[]`, wantLen: 0},
		{name: "one ok", doc: valid, wantLen: 1},
		{name: "oversize refuses", doc: strings.Repeat(" ", maxRateBytes+1), wantErr: "too large"},
		{name: "trailing refuses", doc: `[] trailing`, wantErr: "trailing"},
		{
			name:    "index error names the card",
			doc:     `[` + f115RateCard + `,{"model":"bad","host_version":"v","effective_date":"2026-11-01","currency":"USD","input_per_1k":-1}]`,
			wantErr: "rate card 1:",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseRateCards([]byte(c.doc))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("must fail with %q, got %+v %v", c.wantErr, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("must parse: %v", err)
			}
			if len(got) != c.wantLen {
				t.Fatalf("len = %d, want %d", len(got), c.wantLen)
			}
			if c.wantLen == 1 && (got[0].Model != "beam-free" || got[0].Currency != "USD") {
				t.Fatalf("card identity lost: %+v", got[0])
			}
		})
	}
}
