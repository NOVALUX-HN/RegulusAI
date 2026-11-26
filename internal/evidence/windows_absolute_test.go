package evidence

import "testing"

// TestIsWindowsAbsolute pins the boundary of a path guard.
//
// Mutation testing left seven mutants alive here (CONDITIONALS_BOUNDARY and
// CONDITIONALS_NEGATION on evidence.go:104) because nothing exercised the edges:
// a drive letter at the start or end of a range, a one-character string, or a
// second byte that is not a colon. This function decides whether a task slug can
// escape into an absolute Windows path, so a silent widening is a security
// regression, not a cosmetic one.
func TestIsWindowsAbsolute(t *testing.T) {
	cases := []struct {
		in   string
		want bool
		why  string
	}{
		// Accepted: letter, colon, anything.
		{`C:\x`, true, "drive-anchored with backslash"},
		{`C:/x`, true, "drive-anchored with slash"},
		{`C:x`, true, "drive-anchored relative"},
		{`C:`, true, "two characters is the minimum"},
		{`A:`, true, "lower boundary of A-Z"},
		{`Z:`, true, "upper boundary of A-Z"},
		{`a:`, true, "lower boundary of a-z"},
		{`z:`, true, "upper boundary of a-z"},

		// Rejected: too short, so no colon can follow a letter.
		{``, false, "empty"},
		{`C`, false, "one character"},
		{`:`, false, "one character, colon only"},

		// Rejected: one byte outside each letter range.
		{`@:`, false, "one below 'A'"},
		{`[:`, false, "one above 'Z'"},
		{"`:", false, "one below 'a'"},
		{`{:`, false, "one above 'z'"},

		// Rejected: not a letter at all.
		{`1:`, false, "digit is not a drive letter"},
		{`é:`, false, "multi-byte rune is not a drive letter"},

		// Rejected: the second byte is not a colon.
		{`C;`, false, "semicolon"},
		{`C-`, false, "dash"},
		{`Cx`, false, "letter"},
		{`C\`, false, "backslash without colon"},
	}
	for _, tc := range cases {
		if got := isWindowsAbsolute(tc.in); got != tc.want {
			t.Errorf("isWindowsAbsolute(%q) = %v, want %v (%s)", tc.in, got, tc.want, tc.why)
		}
	}
}
