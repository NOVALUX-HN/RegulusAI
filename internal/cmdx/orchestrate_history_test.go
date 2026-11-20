// History split cover (CRAP paydown sin cambiar comportamiento):
// 5 casos read/decode/load con IO{Dir:tmp}, portables.
package cmdx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadHistoryFileTable(t *testing.T) {
	t.Run("vacia", func(t *testing.T) {
		var errb bytes.Buffer
		raw, code, ok := readHistoryFile("", &errb)
		if !ok || code != 0 || raw != nil {
			t.Fatalf("empty path must be vacia ok, got %v %d %v", raw, code, ok)
		}
		if errb.Len() != 0 {
			t.Fatalf("vacia must stay silent, got %q", errb.String())
		}
	})
	t.Run("read-err", func(t *testing.T) {
		var errb bytes.Buffer
		_, code, ok := readHistoryFile(filepath.Join(t.TempDir(), "missing.json"), &errb)
		if ok || code != 1 {
			t.Fatalf("missing file exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "orchestrate:") {
			t.Fatalf("read failure must name orchestrate, got %q", errb.String())
		}
	})
	t.Run("grande", func(t *testing.T) {
		dir := t.TempDir()
		big := filepath.Join(dir, "big.json")
		if err := os.WriteFile(big, make([]byte, 4*1024*1024+1), 0o644); err != nil {
			t.Fatal(err)
		}
		var errb bytes.Buffer
		_, code, ok := readHistoryFile(big, &errb)
		if ok || code != 1 {
			t.Fatalf("oversized exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "history file too large") {
			t.Fatalf("oversize must name the bound, got %q", errb.String())
		}
	})
	t.Run("ok", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ok.json")
		if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
			t.Fatal(err)
		}
		var errb bytes.Buffer
		raw, code, ok := readHistoryFile(path, &errb)
		if !ok || code != 0 || string(raw) != "[]" {
			t.Fatalf("valid file must load, got %q %d %v", string(raw), code, ok)
		}
	})
}

func TestDecodeLabResultsTable(t *testing.T) {
	t.Run("decode-err", func(t *testing.T) {
		var errb bytes.Buffer
		_, code, ok := decodeLabResults([]byte("{oops"), nil, &errb)
		if ok || code != 1 {
			t.Fatalf("malformed exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "malformed history") {
			t.Fatalf("malformed must name history, got %q", errb.String())
		}
	})
	t.Run("limite", func(t *testing.T) {
		big := "[" + strings.Repeat("{},", 10000) + "{}]"
		var errb bytes.Buffer
		_, code, ok := decodeLabResults([]byte(big), nil, &errb)
		if ok || code != 1 {
			t.Fatalf("over-limit exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "at most 10000") {
			t.Fatalf("limit must name the bound, got %q", errb.String())
		}
	})
	t.Run("ok vacia", func(t *testing.T) {
		var errb bytes.Buffer
		hist, code, ok := decodeLabResults([]byte("[]"), nil, &errb)
		if !ok || code != 0 {
			t.Fatalf("empty array exit = %d ok = %v", code, ok)
		}
		if len(hist) != 0 {
			t.Fatalf("empty array must fold to no history, got %+v", hist)
		}
	})
}

func TestLoadOrchestrateHistoryTable(t *testing.T) {
	write := func(t *testing.T, name, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Run("vacia", func(t *testing.T) {
		var errb bytes.Buffer
		hist, code, ok := loadOrchestrateHistory("", nil, &errb)
		if !ok || code != 0 || len(hist) != 0 {
			t.Fatalf("empty path must be vacia ok, got %+v %d %v", hist, code, ok)
		}
	})
	t.Run("read-err", func(t *testing.T) {
		var errb bytes.Buffer
		_, code, ok := loadOrchestrateHistory(filepath.Join(t.TempDir(), "no.json"), nil, &errb)
		if ok || code != 1 {
			t.Fatalf("exit = %d ok = %v, want 1 false", code, ok)
		}
	})
	t.Run("grande", func(t *testing.T) {
		dir := t.TempDir()
		big := filepath.Join(dir, "big.json")
		if err := os.WriteFile(big, make([]byte, 4*1024*1024+1), 0o644); err != nil {
			t.Fatal(err)
		}
		var errb bytes.Buffer
		_, code, ok := loadOrchestrateHistory(big, nil, &errb)
		if ok || code != 1 {
			t.Fatalf("exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "history file too large") {
			t.Fatalf("must name the size bound, got %q", errb.String())
		}
	})
	t.Run("decode-err", func(t *testing.T) {
		path := write(t, "bad.json", "{oops")
		var errb bytes.Buffer
		_, code, ok := loadOrchestrateHistory(path, nil, &errb)
		if ok || code != 1 {
			t.Fatalf("exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "malformed history") {
			t.Fatalf("must name malformed history, got %q", errb.String())
		}
	})
	t.Run("limite", func(t *testing.T) {
		path := write(t, "many.json", "["+strings.Repeat("{},", 10000)+"{}]")
		var errb bytes.Buffer
		_, code, ok := loadOrchestrateHistory(path, nil, &errb)
		if ok || code != 1 {
			t.Fatalf("exit = %d ok = %v, want 1 false", code, ok)
		}
	})
	t.Run("ok", func(t *testing.T) {
		path := write(t, "ok.json", "[]")
		var errb bytes.Buffer
		hist, code, ok := loadOrchestrateHistory(path, nil, &errb)
		if !ok || code != 0 {
			t.Fatalf("exit = %d ok = %v stderr = %q", code, ok, errb.String())
		}
		if len(hist) != 0 {
			t.Fatalf("empty array must fold empty, got %+v", hist)
		}
	})
}
