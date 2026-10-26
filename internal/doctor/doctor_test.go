package doctor

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// fakeRunner answers with canned output; a cancelled context short-circuits.
type fakeRunner struct {
	out string
	err error
}

func (f fakeRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return f.out, f.err
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		major int
		ok    bool
	}{
		{"plain v2", "2.1.0", 2, true},
		{"v2 with name", "opencode 2.0.19", 2, true},
		{"v2 with version word", "opencode version 2.1.0", 2, true},
		{"v2 with build suffix", "opencode 2.1.0 (9f83ba1)", 2, true},
		{"v1", "opencode 1.14.18", 1, true},
		{"empty", "", 0, false},
		{"name only", "opencode", 0, false},
		{"garbage", "something went wrong", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			major, ok := Classify(tc.in)
			if ok != tc.ok || major != tc.major {
				t.Fatalf("Classify(%q) = (%d, %v), want (%d, %v)", tc.in, major, ok, tc.major, tc.ok)
			}
		})
	}
}

func TestProbeV2Supported(t *testing.T) {
	res := Probe(context.Background(), fakeRunner{out: "opencode 2.1.0"})
	if !res.Supported || res.Major != 2 {
		t.Fatalf("want supported V2, got %+v", res)
	}
	if res.Reason != "" {
		t.Fatalf("supported probe must not carry a reason, got %q", res.Reason)
	}
}

func TestProbeV1RefusedWithGuidance(t *testing.T) {
	res := Probe(context.Background(), fakeRunner{out: "opencode 1.14.18"})
	if res.Supported {
		t.Fatal("V1 must be refused")
	}
	if !strings.Contains(res.Reason, "V1 is not supported") || !strings.Contains(res.Reason, "V2") {
		t.Fatalf("reason must name the V1 refusal and the V2 requirement, got %q", res.Reason)
	}
}

func TestProbeBinaryNotFound(t *testing.T) {
	err := &exec.Error{Name: "opencode", Err: exec.ErrNotFound}
	res := Probe(context.Background(), fakeRunner{err: err})
	if res.Supported {
		t.Fatal("missing binary must be refused")
	}
	if !strings.Contains(res.Reason, "not found on PATH") {
		t.Fatalf("reason must say the binary is missing, got %q", res.Reason)
	}
}

func TestProbeFailedRunIncludesRuntimeOutput(t *testing.T) {
	res := Probe(context.Background(), fakeRunner{
		out: "Error: @opencode/cli's postinstall script was not run.",
		err: errors.New("exit status 1"),
	})
	if res.Supported {
		t.Fatal("failing run must be refused")
	}
	if !strings.Contains(res.Reason, "postinstall") {
		t.Fatalf("reason must include the runtime's own output, got %q", res.Reason)
	}
}

func TestProbeUnparseableOutput(t *testing.T) {
	res := Probe(context.Background(), fakeRunner{out: "opencode"})
	if res.Supported {
		t.Fatal("unparseable output must be refused")
	}
	if !strings.Contains(res.Reason, "could not parse") {
		t.Fatalf("reason must say the output could not be parsed, got %q", res.Reason)
	}
}

func TestProbeContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Probe(ctx, fakeRunner{})
	if res.Supported {
		t.Fatal("cancelled context must be refused")
	}
	if !strings.Contains(res.Reason, "15s") || !strings.Contains(res.Reason, "opencode") {
		t.Fatalf("reason must name the probe budget, got %q", res.Reason)
	}
}
