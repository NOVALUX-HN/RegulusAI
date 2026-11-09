package main

import (
	"errors"
	"net/http"
	"reflect"
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

// H6: parseServeFlags carries the serve flag grammar (defaults, valid
// parses, every invalid branch and --help short-circuit) without binding
// any port, so the full branch matrix runs in milliseconds.
func TestParseServeFlags(t *testing.T) {
	def := serveConfig{port: 17817, bind: "127.0.0.1"}
	cases := []struct {
		name     string
		args     []string
		wantCfg  serveConfig
		wantCode int
		wantOK   bool
	}{
		{"defaults", nil, def, 0, true},
		{"help long", []string{"--help"}, def, 0, false},
		{"help short", []string{"-h"}, def, 0, false},
		{"port valid", []string{"--port=8080"}, serveConfig{port: 8080, bind: "127.0.0.1"}, 0, true},
		{"port spaces trimmed", []string{"--port= 8080 "}, serveConfig{port: 8080, bind: "127.0.0.1"}, 0, true},
		{"port min", []string{"--port=1"}, serveConfig{port: 1, bind: "127.0.0.1"}, 0, true},
		{"port max", []string{"--port=65535"}, serveConfig{port: 65535, bind: "127.0.0.1"}, 0, true},
		{"port abc", []string{"--port=abc"}, def, 2, false},
		{"port empty", []string{"--port="}, def, 2, false},
		{"port zero", []string{"--port=0"}, def, 2, false},
		{"port negative", []string{"--port=-1"}, def, 2, false},
		{"port over max", []string{"--port=65536"}, def, 2, false},
		{"bind valid", []string{"--bind=0.0.0.0"}, serveConfig{port: 17817, bind: "0.0.0.0"}, 0, true},
		{"bind ipv6", []string{"--bind=::1"}, serveConfig{port: 17817, bind: "::1"}, 0, true},
		{"bind spaces trimmed", []string{"--bind= 127.0.0.1 "}, def, 0, true},
		{"bind empty", []string{"--bind="}, def, 2, false},
		{"bind invalid", []string{"--bind=invalid-ip-xyz"}, def, 2, false},
		{"bind hostname rejected", []string{"--bind=localhost"}, def, 2, false},
		{"unknown flag", []string{"--foo"}, def, 2, false},
		{"bare positional", []string{"extra"}, def, 2, false},
		{"port without equals", []string{"--port"}, def, 2, false},
		{"port space form", []string{"--port", "8080"}, serveConfig{port: 8080, bind: "127.0.0.1"}, 0, true},
		{"bind space form", []string{"--bind", "0.0.0.0"}, serveConfig{port: 17817, bind: "0.0.0.0"}, 0, true},
		{"projects space form", []string{"--projects", "/a,/b"}, serveConfig{port: 17817, bind: "127.0.0.1", projects: []string{"/a", "/b"}}, 0, true},
		{"space form missing value at end", []string{"--port=8080", "--bind"}, serveConfig{port: 8080, bind: "127.0.0.1"}, 2, false},
		{"bind without equals", []string{"--bind"}, def, 2, false},
		{"port+bind valid", []string{"--port=8080", "--bind=0.0.0.0"}, serveConfig{port: 8080, bind: "0.0.0.0"}, 0, true},
		{"valid then bogus", []string{"--port=8080", "--bogus"}, serveConfig{port: 8080, bind: "127.0.0.1"}, 2, false},
		{"projects single", []string{"--projects=/a"}, serveConfig{port: 17817, bind: "127.0.0.1", projects: []string{"/a"}}, 0, true},
		{"projects pair", []string{"--projects=/a,/b"}, serveConfig{port: 17817, bind: "127.0.0.1", projects: []string{"/a", "/b"}}, 0, true},
		{"projects spaces trimmed", []string{"--projects= /a , /b "}, serveConfig{port: 17817, bind: "127.0.0.1", projects: []string{"/a", "/b"}}, 0, true},
		{"projects empty", []string{"--projects="}, def, 2, false},
		{"projects blank", []string{"--projects=   "}, def, 2, false},
		{"projects without equals", []string{"--projects"}, def, 2, false},
		{"projects repeated accumulates", []string{"--projects=/a", "--projects=/b"}, serveConfig{port: 17817, bind: "127.0.0.1", projects: []string{"/a", "/b"}}, 0, true},
		{"projects with port", []string{"--port=8080", "--projects=/a,/b"}, serveConfig{port: 8080, bind: "127.0.0.1", projects: []string{"/a", "/b"}}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, code, ok := parseServeFlags(tc.args)
			if code != tc.wantCode || ok != tc.wantOK {
				t.Fatalf("parseServeFlags(%q) = (code %d, ok %v), want (code %d, ok %v)",
					tc.args, code, ok, tc.wantCode, tc.wantOK)
			}
			if ok && !reflect.DeepEqual(got, tc.wantCfg) {
				t.Errorf("parseServeFlags(%q) = %+v, want %+v", tc.args, got, tc.wantCfg)
			}
		})
	}
}

// H6: listenAndServe announces and delegates; the fake serveFn proves both
// the ready path (nil error → 0) and the failure path (error → 1) without
// binding any port.
func TestListenAndServe(t *testing.T) {
	h := http.NewServeMux()
	if got := listenAndServe(serveConfig{port: 17817, bind: "127.0.0.1"}, h,
		func(addr string, _ http.Handler) error {
			if addr != "127.0.0.1:17817" {
				t.Errorf("serveFn addr = %q, want %q", addr, "127.0.0.1:17817")
			}
			return nil
		}); got != 0 {
		t.Errorf("listenAndServe ready = %d, want 0", got)
	}
	if got := listenAndServe(serveConfig{port: 8080, bind: "0.0.0.0"}, h,
		func(string, http.Handler) error { return errors.New("boom") }); got != 1 {
		t.Errorf("listenAndServe failure = %d, want 1", got)
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
		{"projects empty", []string{"--projects="}, 2},
		{"projects blank", []string{"--projects=  ,  "}, 2},
		{"projects without equals", []string{"--projects"}, 2},
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
