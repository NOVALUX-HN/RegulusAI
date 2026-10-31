// Verification of gauntlet receipts (Phase 1, receipt integrity).
//
// Verify recomputes every stored content hash, checks the document version,
// and — for Phase 1 receipts — cross-checks the candidate manifest that
// links the run to its artifacts. Corrupt files are quarantined into a
// corrupt/ subdirectory (moved, never deleted): recovery is re-running the
// gate, not repairing the file. Receipts from an unknown future schema fail
// closed but are never quarantined, so a newer writer's data survives an
// older verifier.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"regulus-ai/internal/manifest"
	"regulus-ai/internal/receipts"
)

// Failure reasons reported by Verify.
const (
	ReasonMalformed          = "malformed"
	ReasonStdoutHashMismatch = "stdout_hash_mismatch"
	ReasonStderrHashMismatch = "stderr_hash_mismatch"
	ReasonUnsupportedSchema  = "unsupported_schema"
	ReasonManifestMissing    = "manifest_missing"
	ReasonManifestMalformed  = "manifest_malformed"
	ReasonManifestUnlisted   = "manifest_unlisted"
	ReasonArtifactMissing    = "artifact_missing"
	ReasonArtifactHashDrift  = "artifact_hash_drift"
)

// Failure is one unverifiable receipt or manifest entry.
type Failure struct {
	File   string
	Reason string
	Detail string
}

// Report is the outcome of one Verify call.
type Report struct {
	Checked     int
	Passed      int
	Legacy      int
	Manifests   int
	Failed      []Failure
	Quarantined []string
}

// Verify checks every receipt under root (or one task's dir), quarantining
// corrupt files when quarantine is true. A task of "" verifies everything.
func Verify(root, task string, quarantine bool) (Report, error) {
	var rep Report
	dirs, err := verifyDirs(root, task)
	if err != nil {
		return rep, err
	}
	quarantined := map[string]bool{}
	for _, dir := range dirs {
		files, err := receiptFiles(dir)
		if err != nil {
			return rep, err
		}
		for _, f := range files {
			rep.Checked++
			// Only corruption (malformed bytes, hash mismatch) is
			// quarantineable: a sound receipt with a broken link (missing
			// manifest, future schema) must stay in place.
			if ok, corrupt := checkReceipt(dir, f, &rep); ok {
				rep.Passed++
			} else if corrupt && quarantine {
				if q, err := quarantineFile(dir, f); err == nil {
					rep.Quarantined = append(rep.Quarantined, q)
					quarantined[filepath.Join(dir, f)] = true
				}
			}
		}
		if n := checkManifests(dir, quarantined, &rep); n > 0 {
			rep.Manifests += n
		}
	}
	sort.Strings(rep.Quarantined)
	return rep, nil
}

func verifyDirs(root, task string) ([]string, error) {
	if task != "" {
		return []string{TaskDir(root, task)}, nil
	}
	pattern := filepath.Join(Dir(root), "*", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var dirs []string
	for _, m := range matches {
		d := filepath.Dir(m)
		if filepath.Base(d) == "corrupt" {
			continue
		}
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

// receiptFiles lists candidate receipt files: *.json except manifests and
// anything already quarantined.
func receiptFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, "manifest-") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// checkReceipt verifies one receipt file. It reports (true, false) when the
// receipt is sound, (false, true) when the file itself is corrupt and may be
// quarantined, and (false, false) when the receipt is sound but something it
// links to is not (missing manifest, future schema): those must stay.
func checkReceipt(dir, name string, rep *Report) (bool, bool) {
	path := filepath.Join(dir, name)
	rec, err := receipts.ReadFile(path)
	if err != nil {
		rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonMalformed, Detail: shortErr(err)})
		return false, true
	}
	if rec.Schema > receipts.ReceiptSchemaVersion {
		rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonUnsupportedSchema, Detail: "schema_version is newer than this verifier"})
		return false, false
	}
	if _, text, sha := captureCheck(rec.Stdout, rec.StdoutSHA); !sha {
		rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonStdoutHashMismatch, Detail: "stored stdout does not match stdout_sha256 (" + text + ")"})
		return false, true
	}
	if _, text, sha := captureCheck(rec.Stderr, rec.StderrSHA); !sha {
		rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonStderrHashMismatch, Detail: "stored stderr does not match stderr_sha256 (" + text + ")"})
		return false, true
	}
	switch {
	case rec.Schema == 0:
		rep.Legacy++
		return true, false
	case rec.Schema != receipts.ReceiptSchemaVersion:
		rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonUnsupportedSchema, Detail: "schema_version is newer than this verifier"})
		return false, false
	}
	if rec.Manifest == "" {
		return true, false
	}
	if checkManifestLink(dir, name, rec, rep) {
		return true, false
	}
	return false, false
}

func captureCheck(text, wantSHA string) (string, string, bool) {
	_, sha := receipts.Capture(text)
	return text, shortSHA(sha), sha == wantSHA
}

func checkManifestLink(dir, name string, rec receipts.Receipt, rep *Report) bool {
	path := filepath.Join(dir, name)
	mpath := filepath.Join(dir, filepath.Base(rec.Manifest))
	m, err := manifest.ReadFile(mpath)
	if err != nil {
		reason := ReasonManifestMissing
		if !os.IsNotExist(err) {
			reason = ReasonManifestMalformed
		}
		rep.Failed = append(rep.Failed, Failure{File: path, Reason: reason, Detail: shortErr(err)})
		return false
	}
	for _, a := range m.Artifacts {
		if a.File != name && a.ReceiptID != rec.ID {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonArtifactMissing, Detail: shortErr(err)})
			return false
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != a.SHA256 {
			rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonArtifactHashDrift, Detail: "receipt changed since the manifest linked it"})
			return false
		}
		return true
	}
	rep.Failed = append(rep.Failed, Failure{File: path, Reason: ReasonManifestUnlisted, Detail: "manifest " + filepath.Base(rec.Manifest) + " does not list this receipt"})
	return false
}

// checkManifests validates every manifest in dir against the files on disk.
// Files quarantined during this same run are reported as quarantined
// consequences, not fresh failures. Returns the manifest count.
func checkManifests(dir string, quarantined map[string]bool, rep *Report) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "manifest-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		n++
		mpath := filepath.Join(dir, e.Name())
		m, err := manifest.ReadFile(mpath)
		if err != nil {
			rep.Failed = append(rep.Failed, Failure{File: mpath, Reason: ReasonManifestMalformed, Detail: shortErr(err)})
			continue
		}
		if m.Schema != manifest.SchemaVersion {
			rep.Failed = append(rep.Failed, Failure{File: mpath, Reason: ReasonUnsupportedSchema, Detail: "manifest schema is newer than this verifier"})
			continue
		}
		for _, a := range m.Artifacts {
			fpath := filepath.Join(dir, filepath.Base(a.File))
			data, err := os.ReadFile(fpath)
			if err != nil {
				if quarantined[fpath] {
					continue
				}
				rep.Failed = append(rep.Failed, Failure{File: mpath, Reason: ReasonArtifactMissing, Detail: "manifest lists " + a.File + " which is absent"})
				continue
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != a.SHA256 {
				rep.Failed = append(rep.Failed, Failure{File: mpath, Reason: ReasonArtifactHashDrift, Detail: a.File + " changed since the manifest linked it"})
			}
		}
	}
	return n
}

// quarantineFile moves a corrupt receipt into corrupt/ (unique name) and
// returns the new path. It never deletes.
func quarantineFile(dir, name string) (string, error) {
	qdir := filepath.Join(dir, "corrupt")
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(qdir, name)
	if _, err := os.Stat(dst); err == nil {
		dst = filepath.Join(qdir, name+".quarantined")
	}
	if err := os.Rename(filepath.Join(dir, name), dst); err != nil {
		return "", err
	}
	return dst, nil
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
