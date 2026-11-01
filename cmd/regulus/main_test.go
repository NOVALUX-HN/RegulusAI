package main

import (
	"strings"
	"testing"
)

func TestServeHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		if got := serve(args); got != 0 {
			t.Errorf("serve(%q) = %d, want 0", args, got)
		}
	}
}

// D3: serve warns visibly when the bind leaves loopback (the read-only
// dashboard still exposes project data), and stays quiet on loopback.
func TestNonLoopbackWarning(t *testing.T) {
	for _, loop := range []string{"127.0.0.1", "::1"} {
		if got := nonLoopbackWarning(loop); got != "" {
			t.Errorf("nonLoopbackWarning(%q) = %q, want empty", loop, got)
		}
	}
	for _, open := range []string{"0.0.0.0", "::", "192.168.1.10"} {
		got := nonLoopbackWarning(open)
		if got == "" {
			t.Errorf("nonLoopbackWarning(%q) = empty, want a warning", open)
			continue
		}
		if !strings.Contains(got, open) {
			t.Errorf("nonLoopbackWarning(%q) = %q, want it to name the address", open, got)
		}
	}
}

func TestServeInvalidArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"unknown flag", []string{"--foo"}, 2},
		{"bare positional", []string{"extra"}, 2},
		{"port without equals", []string{"--port"}, 2},
		{"bind without equals", []string{"--bind"}, 2},
		{"port abc", []string{"--port=abc"}, 2},
		{"port empty", []string{"--port="}, 2},
		{"port zero", []string{"--port=0"}, 2},
		{"port negative", []string{"--port=-1"}, 2},
		{"port over max", []string{"--port=99999"}, 2},
		{"port boundary over", []string{"--port=65536"}, 2},
		{"bind empty", []string{"--bind="}, 2},
		{"bind blank", []string{"--bind=   "}, 2},
		{"bind invalid", []string{"--bind=invalid-ip-xyz"}, 2},
		{"bind hostname rejected", []string{"--bind=localhost"}, 2},
		// Valid parses followed by an invalid arg: covers the
		// port=n / bind= assignment + continue branches without
		// ever reaching ListenAndServe (which would block).
		{"port valid then bogus", []string{"--port=8080", "--bogus"}, 2},
		{"port spaces valid then bogus", []string{"--port= 8080 ", "--bogus"}, 2},
		{"port min then bogus", []string{"--port=1", "--bogus"}, 2},
		{"port max then bogus", []string{"--port=65535", "--bogus"}, 2},
		{"bind valid then bogus", []string{"--bind=127.0.0.1", "--bogus"}, 2},
		{"bind spaces valid then bogus", []string{"--bind= 127.0.0.1 ", "--bogus"}, 2},
		{"port+bind valid then bogus", []string{"--port=8080", "--bind=127.0.0.1", "--bogus"}, 2},
		{"invalid port then help still fails fast", []string{"--port=abc", "--help"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serve(tc.args); got != tc.want {
				t.Errorf("serve(%q) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}
