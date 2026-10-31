// Package manifest builds the reproducible candidate manifest of one
// gauntlet run (Phase 1, trust foundation).
//
// A Git HEAD alone does not identify the exact evaluated content: uncommitted
// changes, untracked files, the gate configuration, tool versions and the
// environment all shape the result. The manifest pins all of them — artifact
// fingerprints, diff hash, modified/untracked inventories, config
// fingerprint, tool versions, command arguments, timestamps, a unique run ID
// and, after the run, task-to-artifact links for every receipt.
//
// The manifest never stores environment values (only a hash of the sorted
// variable names) and never executes anything beyond read-only git
// inspection. Missing git degrades to Available=false instead of failing.
package manifest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// SchemaVersion versions the manifest document. Readers must refuse
// manifests whose schema they do not understand.
const SchemaVersion = 1

// Inventory caps keep the manifest bounded on large trees.
const (
	// MaxFiles caps each inventory list; beyond it truncated flags are set.
	MaxFiles = 200
	// MaxFileHashBytes bounds per-file hashing; bigger files record size only.
	MaxFileHashBytes = 1 << 20
	// MaxDiffBytes bounds the diff bytes hashed; beyond it DiffTruncated is set.
	MaxDiffBytes = 4 << 20
)

// Runner executes one inspection command. It mirrors the gates runner shape
// without importing it (gates imports this package for RunTask).
type Runner interface {
	Run(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error)
}

// GateSpec is one gate's pinned configuration for the manifest.
type GateSpec struct {
	Name            string
	Layer           int
	Command         string
	TimeoutSec      int
	Reproducibility string
	Category        string
}

// Params are the inputs of one manifest build.
type Params struct {
	Root              string
	Task              string
	RunID             string
	RegulusVersion    string
	Contract          string
	ConfigBytes       []byte
	Gates             []GateSpec
	MaxTimeoutSec     int
	AllowDestructive  bool
	AllowlistEnforced bool
	Isolation         string
}

// FileEntry is one modified file's fingerprint.
type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

// GitState is the pinned version-control state of the candidate.
type GitState struct {
	Available          bool        `json:"available"`
	Head               string      `json:"head,omitempty"`
	DiffHash           string      `json:"diff_hash,omitempty"`
	DiffTruncated      bool        `json:"diff_truncated,omitempty"`
	Modified           []FileEntry `json:"modified"`
	ModifiedTruncated  bool        `json:"modified_truncated,omitempty"`
	Untracked          []string    `json:"untracked"`
	UntrackedTruncated bool        `json:"untracked_truncated,omitempty"`
}

// ConfigState pins the evaluated gate configuration.
type ConfigState struct {
	Fingerprint string     `json:"fingerprint"`
	Gates       []GateSpec `json:"gates"`
}

// ToolState pins the toolchain that evaluated the candidate.
type ToolState struct {
	Regulus  string `json:"regulus"`
	Contract string `json:"contract"`
	Go       string `json:"go"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

// EnvState fingerprints the environment by variable names only.
// Values are never read, stored, or hashed.
type EnvState struct {
	NamesSHA256 string `json:"names_sha256"`
	Count       int    `json:"count"`
}

// PolicyState records the execution policy applied to the run.
type PolicyState struct {
	MaxTimeoutSec     int    `json:"max_timeout_sec"`
	AllowDestructive  bool   `json:"allow_destructive"`
	AllowlistEnforced bool   `json:"allowlist_enforced"`
	Isolation         string `json:"isolation"`
}

// ArtifactRef links one written receipt back to the run.
type ArtifactRef struct {
	ReceiptID string `json:"receipt_id"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Gate      string `json:"gate"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}

// Manifest is the reproducible identity of one evaluated candidate.
type Manifest struct {
	Schema           int           `json:"schema"`
	RunID            string        `json:"run_id"`
	Task             string        `json:"task"`
	CreatedAt        time.Time     `json:"created_at"`
	Git              GitState      `json:"git"`
	Config           ConfigState   `json:"config"`
	Tools            ToolState     `json:"tools"`
	Env              EnvState      `json:"env"`
	Policy           PolicyState   `json:"policy"`
	ReproExact       int           `json:"repro_exact"`
	ReproStatistical int           `json:"repro_statistical"`
	ReproUnknown     int           `json:"repro_unknown"`
	Artifacts        []ArtifactRef `json:"artifacts"`
	// Skipped names gates the run did not execute (profile or
	// when-conditions): the skip decision is evidence too.
	Skipped []string `json:"skipped,omitempty"`
}

// NewRunID mints a unique execution ID: run_ + 16 hex chars (64-bit entropy).
func NewRunID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("manifest: cannot mint run id: %w", err)
	}
	return "run_" + hex.EncodeToString(b[:]), nil
}

// ParseReproducibility normalizes a gate reproducibility annotation: empty
// means unknown (honest default), known values pass through, anything else
// is rejected with "".
func ParseReproducibility(s string) string {
	switch s {
	case "":
		return "unknown"
	case "exact", "statistical", "unknown":
		return s
	default:
		return ""
	}
}

// Build pins the candidate described by p. Git inspection failures degrade
// to an unavailable GitState; invalid params or reproducibility values fail.
func Build(ctx context.Context, r Runner, p Params) (Manifest, error) {
	if r == nil {
		return Manifest{}, errors.New("manifest: nil runner")
	}
	if strings.TrimSpace(p.Root) == "" || strings.TrimSpace(p.Task) == "" || strings.TrimSpace(p.RunID) == "" {
		return Manifest{}, errors.New("manifest: root, task and run id are required")
	}
	var gates []GateSpec
	var nExact, nStat, nUnk int
	for _, g := range p.Gates {
		rep := ParseReproducibility(g.Reproducibility)
		if rep == "" {
			return Manifest{}, fmt.Errorf("manifest: gate %q: invalid reproducibility %q", g.Name, g.Reproducibility)
		}
		g.Reproducibility = rep
		gates = append(gates, g)
		switch rep {
		case "exact":
			nExact++
		case "statistical":
			nStat++
		default:
			nUnk++
		}
	}
	m := Manifest{
		Schema:    SchemaVersion,
		RunID:     p.RunID,
		Task:      p.Task,
		CreatedAt: time.Now().UTC(),
		Config: ConfigState{
			Fingerprint: sha256Hex(p.ConfigBytes),
			Gates:       gates,
		},
		Tools: ToolState{
			Regulus:  p.RegulusVersion,
			Contract: p.Contract,
			Go:       runtime.Version(),
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
		},
		Env: fingerEnv(),
		Policy: PolicyState{
			MaxTimeoutSec:     p.MaxTimeoutSec,
			AllowDestructive:  p.AllowDestructive,
			AllowlistEnforced: p.AllowlistEnforced,
			Isolation:         p.Isolation,
		},
		ReproExact:       nExact,
		ReproStatistical: nStat,
		ReproUnknown:     nUnk,
		Artifacts:        []ArtifactRef{},
	}
	m.Git = inspectGit(ctx, r, p.Root)
	return m, nil
}

// Attach links written receipts to the manifest (task-to-artifact links).
func Attach(m *Manifest, refs []ArtifactRef) {
	if m == nil {
		return
	}
	m.Artifacts = append(m.Artifacts, refs...)
}

// FileName returns the manifest file name for a run id.
func FileName(runID string) string {
	return fmt.Sprintf("manifest-%s.json", safeRunFile(runID))
}

// Write stores the manifest atomically (temp file + rename) as
// manifest-<runid>.json under dir and returns the path.
func Write(dir string, m Manifest) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, FileName(m.RunID))
	tmp, err := os.CreateTemp(dir, "manifest-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return path, nil
}

// ReadFile parses one manifest written by Write.
func ReadFile(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%s: malformed manifest: %v", path, err)
	}
	return m, nil
}

func safeRunFile(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "run"
	}
	return b.String()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func fingerEnv() EnvState {
	var names []string
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			names = append(names, kv[:i])
		}
	}
	sort.Strings(names)
	return EnvState{NamesSHA256: sha256Hex([]byte(strings.Join(names, "\n"))), Count: len(names)}
}

func git(ctx context.Context, r Runner, args string) (string, bool) {
	out, _, code, err := r.Run(ctx, args)
	if err != nil || code != 0 {
		return "", false
	}
	return out, true
}

func inspectGit(ctx context.Context, r Runner, root string) GitState {
	st := GitState{Modified: []FileEntry{}, Untracked: []string{}}
	head, ok := git(ctx, r, "git rev-parse --verify HEAD")
	if !ok {
		return st
	}
	st.Available = true
	st.Head = strings.TrimSpace(head)
	if diff, ok := git(ctx, r, "git diff HEAD -- ."); ok {
		if len(diff) > MaxDiffBytes {
			st.DiffTruncated = true
			diff = diff[:MaxDiffBytes]
		}
		st.DiffHash = sha256Hex([]byte(diff))
	}
	porcelain, ok := git(ctx, r, "git status --porcelain=v1 -z --untracked-files=normal -- .")
	if !ok {
		return st
	}
	modified, untracked := parsePorcelain(porcelain)
	for i, p := range modified {
		if i >= MaxFiles {
			st.ModifiedTruncated = true
			break
		}
		st.Modified = append(st.Modified, fingerFile(root, p))
	}
	for i, p := range untracked {
		if i >= MaxFiles {
			st.UntrackedTruncated = true
			break
		}
		st.Untracked = append(st.Untracked, p)
	}
	return st
}

// parsePorcelain parses NUL-separated porcelain v1 entries. Modified holds
// every tracked path that is not unquestioned; untracked holds ?? entries.
// !! (ignored) entries never enter either inventory.
func parsePorcelain(out string) (modified, untracked []string) {
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		e := parts[i]
		if len(e) < 4 {
			continue
		}
		xy, path := e[:2], e[3:]
		if path == "" {
			continue
		}
		switch {
		case xy == "??":
			untracked = append(untracked, path)
		case xy == "!!":
			// ignored — never inventoried
		default:
			modified = append(modified, path)
			if xy[0] == 'R' {
				i++ // rename entries carry the new path in the next slot
			}
		}
	}
	return modified, untracked
}

func fingerFile(root, path string) FileEntry {
	e := FileEntry{Path: path}
	full := filepath.Join(root, path)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return e
	}
	e.Size = info.Size()
	if info.Size() > MaxFileHashBytes {
		return e
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return e
	}
	e.SHA256 = sha256Hex(data)
	return e
}
