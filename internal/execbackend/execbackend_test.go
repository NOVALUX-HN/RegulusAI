package execbackend

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestBackendValid pins the closed vocabulary: a new backend is a new
// execution model, and an unknown value must never pass validation.
func TestBackendValid(t *testing.T) {
	for _, b := range []Backend{Native, WSL, None} {
		if !b.Valid() {
			t.Fatalf("%q must be valid", b)
		}
	}
	for _, b := range []Backend{"", "Windows", "native ", "docker"} {
		if b.Valid() {
			t.Fatalf("%q must be refused", b)
		}
	}
}

// TestResolveNonWindowsIsNative is spec S3 on the Linux side: a host
// without Windows application-control policy keeps the native path
// byte for byte, even when a probe would claim a block.
func TestResolveNonWindowsIsNative(t *testing.T) {
	called := false
	d, err := Resolve(context.Background(), Options{
		GOOS:    "linux",
		Distros: []string{"Ubuntu"},
		Probe: func(context.Context) (bool, string) {
			called = true
			return true, "should not be consulted"
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Backend != Native {
		t.Fatalf("backend = %q, want native", d.Backend)
	}
	if called {
		t.Fatal("probe must not run on a non-Windows host")
	}
	if d.Reason == "" {
		t.Fatal("reason must explain the decision")
	}
}

// TestResolveWindowsHealthyStaysNative protects existing behaviour: a
// Windows host whose probe is inconclusive or absent must not be moved
// into a sandbox.
func TestResolveWindowsHealthyStaysNative(t *testing.T) {
	tests := []struct {
		name  string
		probe BlockProbe
	}{
		{"no probe", nil},
		{"probe not blocked", func(context.Context) (bool, string) { return false, "binary executes" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Resolve(context.Background(), Options{
				GOOS:    "windows",
				Probe:   tc.probe,
				Distros: []string{"Ubuntu"},
			})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if d.Backend != Native {
				t.Fatalf("backend = %q, want native", d.Backend)
			}
			if d.Reason == "" {
				t.Fatal("reason must explain the decision")
			}
		})
	}
}

// TestResolveBlockedPicksWSL covers the affected host: a proven block
// with an installed distro resolves to the sandbox.
func TestResolveBlockedPicksWSL(t *testing.T) {
	blocked := func(context.Context) (bool, string) { return true, "SAC enforcement" }

	t.Run("first distro", func(t *testing.T) {
		d, err := Resolve(context.Background(), Options{
			GOOS:    "windows",
			Probe:   blocked,
			Distros: []string{"Debian", "Ubuntu"},
		})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if d.Backend != WSL || d.Distro != "Debian" {
			t.Fatalf("got %q/%q, want wsl/Debian", d.Backend, d.Distro)
		}
		if !strings.Contains(d.Reason, "SAC enforcement") {
			t.Fatalf("reason must carry the probe evidence, got %q", d.Reason)
		}
	})

	t.Run("preferred distro", func(t *testing.T) {
		d, err := Resolve(context.Background(), Options{
			GOOS:    "windows",
			Probe:   blocked,
			Distros: []string{"Debian", "Ubuntu"},
			Distro:  "Ubuntu",
		})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if d.Backend != WSL || d.Distro != "Ubuntu" {
			t.Fatalf("got %q/%q, want wsl/Ubuntu", d.Backend, d.Distro)
		}
	})
}

// TestResolveBlockedWithoutDistroIsNone pins the honest failure: no
// sandbox means BLOCKED upstream, never a silent skip presented as a
// pass.
func TestResolveBlockedWithoutDistroIsNone(t *testing.T) {
	d, err := Resolve(context.Background(), Options{
		GOOS:  "windows",
		Probe: func(context.Context) (bool, string) { return true, "policy refusal" },
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Backend != None {
		t.Fatalf("backend = %q, want none", d.Backend)
	}
	if !strings.Contains(d.Reason, "policy") {
		t.Fatalf("reason must name the cause, got %q", d.Reason)
	}
}

// TestResolveOverride: governance beats probing, and a typo is an
// error instead of a silent native selection.
func TestResolveOverride(t *testing.T) {
	blocked := func(context.Context) (bool, string) { return true, "policy refusal" }

	tests := []struct {
		name    string
		opts    Options
		want    Backend
		wantErr bool
	}{
		{"native beats block", Options{Override: "native", GOOS: "windows", Probe: blocked}, Native, false},
		{"none explicit", Options{Override: "none", GOOS: "windows", Probe: blocked}, None, false},
		{"wsl with distro", Options{Override: "wsl", GOOS: "windows", Distros: []string{"Ubuntu"}}, WSL, false},
		{"wsl without distro", Options{Override: "wsl", GOOS: "windows"}, None, false},
		{"auto falls through", Options{Override: "auto", GOOS: "windows", Probe: blocked, Distros: []string{"Ubuntu"}}, WSL, false},
		{"typo is refused", Options{Override: "windwos"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Resolve(context.Background(), tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if d.Backend != tc.want {
				t.Fatalf("backend = %q, want %q", d.Backend, tc.want)
			}
		})
	}
}

// TestWrap is the safety property of S3: native argv must come back
// unchanged, byte for byte, or every existing caller changes meaning.
func TestWrap(t *testing.T) {
	argv := []string{"go", "test", "./internal/pxexec/"}
	got, err := Decision{Backend: Native}.Wrap(argv)
	if err != nil {
		t.Fatalf("native wrap: %v", err)
	}
	if !reflect.DeepEqual(got, argv) {
		t.Fatalf("native argv changed: %v", got)
	}

	got, err = Decision{Backend: WSL, Distro: "Ubuntu"}.Wrap(argv)
	if err != nil {
		t.Fatalf("wsl wrap: %v", err)
	}
	want := append([]string{"wsl.exe", "-d", "Ubuntu", "-e"}, argv...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wsl argv = %v, want %v", got, want)
	}

	if _, err := (Decision{Backend: WSL}).Wrap(argv); err == nil {
		t.Fatal("wsl without distro must refuse")
	}
	if _, err := (Decision{Backend: None}).Wrap(argv); err == nil {
		t.Fatal("none must refuse instead of pretending")
	}
	if _, err := (Decision{Backend: Native}).Wrap(nil); err == nil {
		t.Fatal("empty argv must refuse")
	}
}

// TestIsPolicySignal keeps the trigger narrow: only explicit
// application-control refusals count, so a healthy host is never moved
// into a sandbox by a generic permission error.
func TestIsPolicySignal(t *testing.T) {
	blocked := []string{
		"fork/exec C:\\tmp\\x.test.exe: Una directiva de Control de aplicaciones bloqueó este archivo.",
		"BLOCKED THIS FILE by policy",
		"Code Integrity determined that the image does not meet requirements",
		"Smart App Control blocked the launch",
	}
	for _, text := range blocked {
		if _, ok := IsPolicySignal(text); !ok {
			t.Fatalf("%q must be a policy signal", text)
		}
	}
	allowed := []string{
		"",
		"exit status 1",
		"Access is denied",
		"permission denied",
		"no such file or directory",
	}
	for _, text := range allowed {
		if marker, ok := IsPolicySignal(text); ok {
			t.Fatalf("%q must not be a policy signal (matched %q)", text, marker)
		}
	}
}

// TestParseDistros: `wsl.exe -l -q` output arrives with NUL bytes,
// CRLF endings and trailing blanks depending on the build.
func TestParseDistros(t *testing.T) {
	out := "U\x00b\x00u\x00n\x00t\x00u\x00\r\n\x00docker-desktop\r\n\r\n   \n"
	got := parseDistros(out)
	want := []string{"Ubuntu", "docker-desktop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDistros = %v, want %v", got, want)
	}
	if got := parseDistros(""); len(got) != 0 {
		t.Fatalf("empty output must yield no distros, got %v", got)
	}
}

// TestHostProbeNonWindows: the probe is a Windows-only mechanism, so
// on Linux it must never claim a block.
func TestHostProbeNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows assertion")
	}
	blocked, reason := HostProbe(context.Background(), func(context.Context, []string) (string, error) {
		t.Fatal("probe must not launch anything on a non-Windows host")
		return "", nil
	}, "go")
	if blocked {
		t.Fatalf("blocked = true on %s: %s", runtime.GOOS, reason)
	}
}

// TestHostProbeWindowsPaths exercises the probe's decisions through
// the injected runner, so the Windows logic is covered from any host.
func TestHostProbeWindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only assertion")
	}

	blocked, reason := HostProbe(context.Background(), func(_ context.Context, argv []string) (string, error) {
		if len(argv) > 1 && argv[1] == "build" {
			return "", nil
		}
		return "Una directiva de Control de aplicaciones bloqueó este archivo.", errors.New("exit status 1")
	}, "go")
	if !blocked {
		t.Fatalf("policy refusal must report blocked, got %q", reason)
	}

	blocked, reason = HostProbe(context.Background(), func(_ context.Context, argv []string) (string, error) {
		if len(argv) > 1 && argv[1] == "build" {
			return "", nil
		}
		return "", errors.New("exit status 0")
	}, "go")
	if blocked {
		t.Fatalf("successful launch must not report a block: %q", reason)
	}
}
