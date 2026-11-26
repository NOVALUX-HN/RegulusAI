package almanac

import "testing"

// TestContextScopeNarrowsByHostAndEnvironment is F18.3's retrieval half: the
// host and environment dimensions the record gained in F18.2 must be usable as
// a retrieval scope, or the dimension is stored and never honoured.
func TestContextScopeNarrowsByHostAndEnvironment(t *testing.T) {
	linux := Observation{Host: "linux"}
	windows := Observation{Host: "windows"}
	unscoped := Observation{}

	byHost := ContextScope{Host: "linux"}
	if !byHost.matches(linux) {
		t.Fatal("a claim established on linux must match a linux scope")
	}
	if byHost.matches(windows) {
		t.Fatal("a claim established on windows must not match a linux scope")
	}
	if !byHost.matches(unscoped) {
		t.Fatal("an unscoped claim makes no claim about another context, so it stays eligible")
	}

	wsl := Observation{Environment: "wsl"}
	native := Observation{Environment: "native"}
	byEnv := ContextScope{Environment: "wsl"}
	if !byEnv.matches(wsl) {
		t.Fatal("a wsl claim must match a wsl scope")
	}
	if byEnv.matches(native) {
		t.Fatal("a native claim must not match a wsl scope")
	}
	if !byEnv.matches(unscoped) {
		t.Fatal("an unscoped claim stays eligible")
	}
}

// TestContextScopeHostIsCaseInsensitive: hosts are identifiers, not prose.
func TestContextScopeHostIsCaseInsensitive(t *testing.T) {
	s := ContextScope{Host: "LINUX"}
	if !s.matches(Observation{Host: "linux"}) {
		t.Fatal("host matching must ignore case")
	}
}

// TestContextScopeActiveWithOnlyHostOrEnvironment: a scope carrying only the new
// dimensions is still an active scope, otherwise it would be silently ignored.
func TestContextScopeActiveWithOnlyHostOrEnvironment(t *testing.T) {
	if !(ContextScope{Host: "linux"}).active() {
		t.Fatal("a host-only scope must be active")
	}
	if !(ContextScope{Environment: "wsl"}).active() {
		t.Fatal("an environment-only scope must be active")
	}
	if (ContextScope{}).active() {
		t.Fatal("an empty scope is not active")
	}
}
