package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NOV-82: token resolution precedence (--token > --token-file >
// REGULUS_DASH_TOKEN > none) and loopback detection for the guard.
func TestIsLoopbackBind(t *testing.T) {
	cases := []struct {
		bind string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"::", false},
		{"192.168.1.10", false},
		// parseServeFlags only accepts IPs; hostnames fail closed here.
		{"localhost", false},
	}
	for _, tc := range cases {
		if got := isLoopbackBind(tc.bind); got != tc.want {
			t.Errorf("isLoopbackBind(%q) = %v, want %v", tc.bind, got, tc.want)
		}
	}
}

func TestResolveServeToken(t *testing.T) {
	if got, err := resolveServeToken(serveConfig{token: " flag "}); err != nil || got != "flag" {
		t.Fatalf("explicit token = %q, %v; want trimmed flag", got, err)
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "token")
	if err := os.WriteFile(f, []byte(" file-token \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveServeToken(serveConfig{token: "flag", tokenFile: f}); err != nil || got != "flag" {
		t.Fatalf("flag must win over file: got %q, %v", got, err)
	}
	if got, err := resolveServeToken(serveConfig{tokenFile: f}); err != nil || got != "file-token" {
		t.Fatalf("file token = %q, %v", got, err)
	}
	t.Setenv("REGULUS_DASH_TOKEN", "env-token")
	if got, err := resolveServeToken(serveConfig{}); err != nil || got != "env-token" {
		t.Fatalf("env token = %q, %v", got, err)
	}
	if got, err := resolveServeToken(serveConfig{tokenFile: filepath.Join(dir, "missing")}); err == nil || got != "" {
		t.Fatalf("missing file must fail, got %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty"), []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveServeToken(serveConfig{tokenFile: filepath.Join(dir, "empty")}); err == nil || got != "" {
		t.Fatalf("empty file must fail, got %q, %v", got, err)
	}
}

func TestGenerateServeToken(t *testing.T) {
	a, err := generateServeToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := generateServeToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(a) != 64 || a == b {
		t.Fatalf("tokens must be 64 unique hex chars, got %q vs %q", a, b)
	}
	for _, c := range a {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("token must be hex, got %q", a)
		}
	}
}
