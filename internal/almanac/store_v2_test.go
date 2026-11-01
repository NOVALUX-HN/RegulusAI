package almanac

// M2 phase 1: index v2 (sorted offsets + exact verify), atomic reindex,
// legacy v1 refusal, lockfile. One concern per test.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// seedShuffledLog writes three observations whose log order is NOT id
// order (an import preserving a foreign older id), returning them in
// log order. The index must still sort records by id.
func seedShuffledLog(t *testing.T, root string) []Observation {
	t.Helper()
	newest, err := Save(root, &Observation{Type: TypeDecision, Title: "newest first", Content: "saved first"}, t2)
	if err != nil {
		t.Fatal(err)
	}
	// A foreign older id appended after (import preserves ids verbatim).
	old := Observation{ID: "obs_aaa000", Type: TypeDiscovery, Title: "older second", Content: "imported after", SavedAt: t0}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(File(root), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	mid, err := Save(root, &Observation{Type: TypeDiscovery, Title: "mid third", Content: "saved last", TopicKey: "k1", Domain: "go"}, t1)
	if err != nil {
		t.Fatal(err)
	}
	return []Observation{newest, old, mid}
}

func TestV2RecordsSortedWithExactOffsets(t *testing.T) {
	root := t.TempDir()
	logged := seedShuffledLog(t, root)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	entries, hdr, err := LoadIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Version != IndexVersion || IndexVersion != 2 {
		t.Fatalf("header version = %d, want IndexVersion 2", hdr.Version)
	}
	if len(entries) != len(logged) {
		t.Fatalf("entries = %d, want %d", len(entries), len(logged))
	}
	if !sort.SliceIsSorted(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID }) {
		t.Fatal("v2 records must be sorted by id ascending (ordenado)")
	}
	// Every entry seeks back to its exact observation (verify exacto).
	byID := map[string]Observation{}
	for _, o := range logged {
		byID[o.ID] = o
	}
	for _, e := range entries {
		got, err := LookupByID(root, e.ID)
		if err != nil {
			t.Fatalf("LookupByID(%q): %v", e.ID, err)
		}
		want := byID[e.ID]
		if got.Title != want.Title || got.Content != want.Content || got.TopicKey != want.TopicKey || got.Domain != want.Domain {
			t.Fatalf("LookupByID(%q) = %+v, want %+v", e.ID, got, want)
		}
	}
	// Buckets ride the record (no JSONL read to filter them).
	for _, e := range entries {
		if e.ID == byID[e.ID].ID && e.TopicKey != byID[e.ID].TopicKey {
			t.Fatalf("record %q buckets not pinned: %+v", e.ID, e)
		}
	}
}

// craftV1Index builds a retired v1-format file (vectors only, no
// offsets) with the real header writer shape: magic + version 1 +
// embedder + dim + count + sha + (id + vector) records.
func craftV1Index(embedder string, id string, vec [EmbedDim]int8, sum [32]byte) []byte {
	buf := &bytes.Buffer{}
	buf.Write(indexMagic[:])
	var v [4]byte
	binary.LittleEndian.PutUint32(v[:], 1)
	buf.Write(v[:])
	var n [2]byte
	binary.LittleEndian.PutUint16(n[:], uint16(len(embedder)))
	buf.Write(n[:])
	buf.WriteString(embedder)
	var d [4]byte
	binary.LittleEndian.PutUint32(d[:], uint32(EmbedDim))
	buf.Write(d[:])
	var c [4]byte
	binary.LittleEndian.PutUint32(c[:], 1)
	buf.Write(c[:])
	buf.Write(sum[:])
	var il [2]byte
	binary.LittleEndian.PutUint16(il[:], uint16(len(id)))
	buf.Write(il[:])
	buf.WriteString(id)
	for _, b := range vec {
		buf.WriteByte(byte(b))
	}
	return buf.Bytes()
}

func TestV1IndexRefusesFriendly(t *testing.T) {
	root := t.TempDir()
	saved, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(File(root))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	v1 := craftV1Index(EmbedderID, saved.ID, EmbedObservation(saved), sum)
	if err := os.MkdirAll(filepath.Dir(IndexPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(IndexPath(root), v1, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = LoadIndex(root)
	if !errors.Is(err, ErrHeaderMismatch) {
		t.Fatalf("retired v1 must refuse with ErrHeaderMismatch, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "v1") {
		t.Fatalf("v1 refusal must name v1 amably, got %q", err)
	}
	if err != nil && strings.Contains(err.Error(), "truncated") {
		t.Fatalf("v1 refusal must never read as truncation, got %q", err)
	}
	if state, serr := IndexState(root); serr != nil || state != "stale" {
		t.Fatalf("v1 index: IndexState = %q/%v, want stale (same repair)", state, serr)
	}
	// Repair is one reindex to v2.
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	if _, hdr, err := LoadIndex(root); err != nil || hdr.Version != 2 {
		t.Fatalf("reindex must heal v1 to v2, hdr=%+v err=%v", hdr, err)
	}
}

func TestReindexAtomicNoLeftovers(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	left, err := filepath.Glob(filepath.Join(filepath.Dir(IndexPath(root)), "retrieval-*.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("atomic reindex must leave no tmp behind, found %v", left)
	}
	if _, _, err := LoadIndex(root); err != nil {
		t.Fatalf("reindexed file must load: %v", err)
	}
	// Twice on the same log → byte-identical (pure function, M1 S1 kept).
	first, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("reindex must stay a pure function of the log (byte-identical twice)")
	}
}

func TestScanLogMirrorsAll(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "one", Content: "1"}, t0); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(File(root), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"", "   ", "{malformed json", "{\"type\":\"rant\"}"} {
		if _, err := f.WriteString(line + "\n"); err != nil {
			f.Close()
			t.Fatal(err)
		}
	}
	f.Close()
	if _, err := Save(root, &Observation{Type: TypeDiscovery, Title: "two", Content: "2"}, t1); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(File(root))
	if err != nil {
		t.Fatal(err)
	}
	lines := scanLog(raw)
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != len(all) {
		t.Fatalf("scanLog covers %d, All covers %d — must mirror", len(lines), len(all))
	}
	for i := range lines {
		if lines[i].obs.ID != all[i].ID {
			t.Fatalf("line %d: scanLog %q vs All %q", i, lines[i].obs.ID, all[i].ID)
		}
		// The recorded window re-reads to the same observation.
		recs, err := readByOffsets(root, []IndexEntry{{
			ID: lines[i].obs.ID, Offset: lines[i].offset, Length: lines[i].length,
		}})
		if err != nil {
			t.Fatal(err)
		}
		if recs[0].ID != all[i].ID || recs[0].Title != all[i].Title {
			t.Fatalf("offset window %d re-reads %+v, want %+v", i, recs[0], all[i])
		}
	}
}

func TestIndexStatsV2(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	ver, count, sha, err := IndexStats(root)
	if err != nil {
		t.Fatal(err)
	}
	if ver != 2 || count != 1 || len(sha) != 64 {
		t.Fatalf("IndexStats = (%d,%d,%q), want (2,1,64-hex)", ver, count, sha)
	}
	if _, _, _, err := IndexStats(t.TempDir()); !os.IsNotExist(err) {
		t.Fatalf("missing index must report missing, got %v", err)
	}
}

func TestLookupByIDSkewAfterPruneWithoutReindex(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "first", Content: "a"}, t0); err != nil {
		t.Fatal(err)
	}
	second, err := Save(root, &Observation{Type: TypeDecision, Title: "second", Content: "b"}, t1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	// Prune the FIRST line without reindexing: the survivor shifts offsets.
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPrune(root, []Observation{all[0]}); err != nil {
		t.Fatal(err)
	}
	if _, err := LookupByID(root, second.ID); !errors.Is(err, ErrIndexSkew) {
		t.Fatalf("shifted offsets must signal ErrIndexSkew (fallback), got %v", err)
	}
	// Reindex heals the skew: the survivor reads, the pruned id is gone.
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	got, err := LookupByID(root, second.ID)
	if err != nil {
		t.Fatalf("reindex must heal skew: %v", err)
	}
	if got.ID != second.ID || got.Title != "second" {
		t.Fatalf("healed LookupByID = %+v, want the prune survivor", got)
	}
}
