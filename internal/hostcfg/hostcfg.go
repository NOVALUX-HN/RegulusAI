// Host config ownership (NOV-69, S4): install, sync and uninstall
// must never overwrite foreign configuration. A file is managed only
// when it carries the regulus marker; anything else is foreign and
// stops the operation with its bytes untouched (JSONC comments,
// user permissions and unknown keys included: the merger never
// parses what it does not own). Every write is preceded by a
// timestamped backup; Restore rolls back to it.
package hostcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Marker claims a file as regulus-managed. It is a line comment so it
// survives in TOML, shell and markdown; JSON files are never marked
// inline (no comment syntax) and stay foreign unless adopted through
// an explicit adopt step the user runs once.
const Marker = "# regulus-managed"

// Plan describes what an install/sync/uninstall would do.
type Plan struct {
	Path    string
	Missing bool
	Managed bool
	Foreign bool
	Backup  string
}

// Inspect classifies one config path without writing anything.
func Inspect(path string) (Plan, error) {
	p := Plan{Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			p.Missing = true
			return p, nil
		}
		return p, err
	}
	if hasMarker(string(data)) {
		p.Managed = true
		return p, nil
	}
	p.Foreign = true
	return p, nil
}

func hasMarker(body string) bool {
	for _, ln := range strings.Split(body, "\n") {
		if strings.TrimSpace(ln) == Marker {
			return true
		}
	}
	return false
}

// Backup copies path to path+".bak-<UTC timestamp>" and returns the
// backup path. Missing files need no backup.
func Backup(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	bak := fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(filepath.Dir(bak), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(bak, data, 0o644); err != nil {
		return "", err
	}
	return bak, nil
}

// Install writes a managed file: fresh when missing, in place (after
// backup) when managed. Foreign files refuse: the error names the
// preserved path and no byte changes.
func Install(path, body string) (string, error) {
	p, err := Inspect(path)
	if err != nil {
		return "", err
	}
	if p.Foreign {
		return "", fmt.Errorf("foreign config preserved at %s (not managed by regulus; adopt it explicitly or back it up yourself)", path)
	}
	content := Marker + "\n" + strings.TrimRight(body, "\n") + "\n"
	if p.Missing {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", err
		}
		return "", nil
	}
	bak, err := Backup(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return bak, nil
}

// Restore rolls a path back to its backup.
func Restore(path, backup string) error {
	data, err := os.ReadFile(backup)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Uninstall removes a managed file (after backup) and reports the
// backup path. Foreign or missing files are left alone.
func Uninstall(path string) (string, error) {
	p, err := Inspect(path)
	if err != nil {
		return "", err
	}
	if p.Missing || p.Foreign {
		return "", nil
	}
	bak, err := Backup(path)
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return bak, nil
}
