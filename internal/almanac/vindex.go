package almanac

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// Retrieval index format v2 (M2, docs/adr/M1-01-sqlite-vs-stdlib.md):
// header {magic, version=2, embedder, dim, count, sha256-of-source-JSONL}
// + count × records sorted by id ascending ("ordenado"):
// (id + file offset u64 + line length u32 + topic + domain + type +
// lesson-dimension + dim × int8 vector).
//
// The offset table turns id and bucket lookups into bounded seeks: the
// indexed path reads only the candidate lines and verifies each one
// exacto (re-parse + id match) instead of full-scanning the JSONL.
// Reindex stays a pure function of observations.jsonl: twice on the
// same log → byte-identical file. A header mismatch (magic, version,
// embedder, dim) is never trusted — callers rebuild. A v1 file (no
// offsets) refuses with ErrHeaderMismatch, never "truncated".

// IndexMagic identifies a retrieval.bin file.
var indexMagic = [4]byte{'R', 'V', 'E', 'C'}

// IndexVersion is the current header version (M2: v2 adds the sorted
// offset table; v1 files refuse with ErrHeaderMismatch and rebuild).
const IndexVersion uint32 = 2

// retiredIndexVersion is the pre-M2 layout (vectors only, no offsets).
const retiredIndexVersion uint32 = 1

// ErrHeaderMismatch signals an untrusted index: magic, version,
// embedder or dim differs — rebuild, never read. A retired v1 file
// lands here too (friendly: it names the repair, not "truncated").
var ErrHeaderMismatch = errors.New("almanac: retrieval index header mismatch — rebuild with `regulus memory reindex`")

// ErrNoLog signals there is no observation log to index yet.
var ErrNoLog = errors.New("almanac: no observations log yet — save an observation first")

// ErrIndexSkew signals the index disagrees with observations.jsonl
// (offset drift after a prune/import without reindex, torn read):
// callers fall back to the legacy full-scan path, never fail.
var ErrIndexSkew = errors.New("almanac: retrieval index disagrees with observations.jsonl — run `regulus memory reindex`")

// IndexEntry is one indexed observation: its identity, its exact byte
// window in observations.jsonl, its index-coverable buckets
// (topic/domain/type/lesson-dimension; system derives from type), and
// its vector for the cosine channel.
type IndexEntry struct {
	ID        string
	Offset    uint64
	Length    uint32
	TopicKey  string
	Domain    string
	Type      string
	Dimension string
	Vec       [EmbedDim]int8
}

// IndexHeader is the parsed retrieval.bin header.
type IndexHeader struct {
	Version   uint32
	Embedder  string
	Dim       int
	Count     int
	SourceSHA [32]byte
}

// SourceSHAHex renders the pinned source sha256 as hex.
func (h IndexHeader) SourceSHAHex() string {
	return hex.EncodeToString(h.SourceSHA[:])
}

// IndexPath returns the derived artifact path under a project root.
func IndexPath(root string) string {
	return filepath.Join(root, ".regulus", "almanac", "retrieval.bin")
}

// sha256Hex hashes bytes to lowercase hex.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// logLine is one readable JSONL line with its exact byte window.
type logLine struct {
	obs    Observation
	offset uint64
	length uint32
}

// scanLog parses the raw log mirroring All: blank lines skipped,
// malformed lines skipped with the same stderr warning, every readable
// line kept with its byte window (offset + segment length without the
// newline). No line-length cap (All caps at 1MB via bufio.Scanner);
// only pathological >1MB lines diverge, and then Reindex covers more,
// never less.
func scanLog(raw []byte) []logLine {
	var out []logLine
	var off uint64
	ln := 0
	rest := raw
	for len(rest) > 0 {
		var seg []byte
		var step uint64
		if i := bytes.IndexByte(rest, '\n'); i < 0 {
			seg, step = rest, uint64(len(rest))
			rest = nil
		} else {
			seg, step = rest[:i], uint64(i+1)
			rest = rest[i+1:]
		}
		ln++
		trimmed := bytes.TrimSpace(seg)
		if len(trimmed) == 0 {
			off += step
			continue
		}
		var o Observation
		if err := json.Unmarshal(trimmed, &o); err != nil {
			log.Printf("almanac: skipping malformed observation at line %d: %v", ln, err)
			off += step
			continue
		}
		out = append(out, logLine{obs: o, offset: off, length: uint32(len(seg))})
		off += step
	}
	return out
}

// Reindex rebuilds retrieval.bin as a pure function of
// observations.jsonl. It returns the observation count and the hex
// sha256 of the source log.
func Reindex(root string) (int, string, error) {
	return ReindexWith(root, "")
}

// ReindexWith rebuilds retrieval.bin with the named embedder ("" means
// the default hash-ngram-v1). An unknown embedder id refuses before any
// write; the header pins {embedder, dim, version} so a later load with a
// different encoder refuses with ErrHeaderMismatch instead of reading
// foreign vectors.
//
// Atomicity: the lockfile serializes writers (Save/Reindex/ApplyPrune)
// and the file lands via tmp+Rename — never a direct WriteFile, so a
// crash leaves either the old index or the new one, never a torn one.
// No fsync/WAL: tmp+Rename is the whole transaction (ADR M1).
func ReindexWith(root, embedderID string) (int, string, error) {
	emb, err := LookupEmbedder(embedderID)
	if err != nil {
		return 0, "", err
	}
	var count int
	var sha string
	if err := withStoreLock(root, func() error {
		raw, err := os.ReadFile(File(root))
		if os.IsNotExist(err) {
			return ErrNoLog
		}
		if err != nil {
			return err
		}
		lines := scanLog(raw)
		if len(lines) == 0 {
			return ErrNoLog
		}
		// Ordered index: sort by id ascending. Log order is already
		// id order for pure appends, but imports preserve foreign ids
		// — sorting keeps the layout deterministic either way.
		sorted := append([]logLine(nil), lines...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].obs.ID < sorted[j].obs.ID })
		sum := sha256.Sum256(raw)
		buf := newIndexBuffer(emb.ID(), len(sorted), sum)
		for _, l := range sorted {
			writeRecord(buf, l, emb)
		}
		if err := os.MkdirAll(filepath.Dir(IndexPath(root)), 0o755); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(IndexPath(root)), "retrieval-*.bin")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		if _, err := tmp.Write(buf.Bytes()); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpName)
			return err
		}
		if err := os.Rename(tmpName, IndexPath(root)); err != nil {
			os.Remove(tmpName)
			return err
		}
		count, sha = len(sorted), hex.EncodeToString(sum[:])
		return nil
	}); err != nil {
		return 0, "", err
	}
	return count, sha, nil
}

// newIndexBuffer writes the fixed header prefix.
func newIndexBuffer(embedder string, count int, sum [32]byte) *bytes.Buffer {
	buf := &bytes.Buffer{}
	buf.Write(indexMagic[:])
	writeU32(buf, IndexVersion)
	writeSized(buf, embedder)
	writeU32(buf, uint32(EmbedDim))
	writeU32(buf, uint32(count))
	buf.Write(sum[:])
	return buf
}

// writeRecord appends one ordered-index record: identity + byte window
// + buckets + vector.
func writeRecord(buf *bytes.Buffer, l logLine, emb Embedder) {
	writeSized(buf, l.obs.ID)
	writeU64(buf, l.offset)
	writeU32(buf, l.length)
	writeSized(buf, l.obs.TopicKey)
	writeSized(buf, l.obs.Domain)
	writeSized(buf, l.obs.Type)
	dim := ""
	if l.obs.Lesson != nil {
		dim = l.obs.Lesson.Dimension
	}
	writeSized(buf, dim)
	vec := emb.EmbedObservation(l.obs)
	for _, b := range vec {
		buf.WriteByte(byte(b))
	}
}

// writeSized appends a u16-length-prefixed string.
func writeSized(buf *bytes.Buffer, s string) {
	writeU16(buf, uint16(len(s)))
	buf.WriteString(s)
}

// LookupByID reads one observation through the offset table: binary
// search by id (records are sorted), seek to its byte window, re-parse
// and verify the id matches exacto. Skew (log rewritten without
// reindex) reports ErrIndexSkew — the caller falls back to full-scan,
// never fails.
func LookupByID(root, id string) (Observation, error) {
	entries, _, err := LoadIndex(root)
	if err != nil {
		return Observation{}, err
	}
	i := sort.Search(len(entries), func(i int) bool { return entries[i].ID >= id })
	if i >= len(entries) || entries[i].ID != id {
		return Observation{}, fmt.Errorf("almanac: no observation %q in the index — run `regulus memory reindex`", id)
	}
	recs, err := readByOffsets(root, []IndexEntry{entries[i]})
	if err != nil {
		return Observation{}, err
	}
	return recs[0], nil
}

// readByOffsets reads exactly the given records' byte windows in one
// open: seek + bounded read + parse + id verify each. Any skew (short
// read, unparsable line, id drift) aborts with ErrIndexSkew so the
// caller falls back to the legacy path. A missing log is a real error,
// not skew (legacy All would fail the same way).
func readByOffsets(root string, recs []IndexEntry) ([]Observation, error) {
	if len(recs) == 0 {
		return nil, nil
	}
	f, err := os.Open(File(root))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make([]Observation, 0, len(recs))
	for _, e := range recs {
		if _, err := f.Seek(int64(e.Offset), io.SeekStart); err != nil {
			return nil, fmt.Errorf("%w: seek %q: %v", ErrIndexSkew, e.ID, err)
		}
		seg := make([]byte, e.Length)
		if _, err := io.ReadFull(f, seg); err != nil {
			return nil, fmt.Errorf("%w: short read %q: %v", ErrIndexSkew, e.ID, err)
		}
		var o Observation
		if err := json.Unmarshal(bytes.TrimSpace(seg), &o); err != nil {
			return nil, fmt.Errorf("%w: unparsable line %q: %v", ErrIndexSkew, e.ID, err)
		}
		if o.ID != e.ID {
			return nil, fmt.Errorf("%w: id drift at offset %d (want %q, got %q)", ErrIndexSkew, e.Offset, e.ID, o.ID)
		}
		out = append(out, o)
	}
	return out, nil
}

// IndexState reports the retrieval index freshness for status surfaces:
// "fresh" (hybrid ranking active), "stale" (the log changed since the
// last reindex, the index is corrupt, or a retired v1 — same repair:
// `regulus memory reindex`), "missing" (observations exist but
// retrieval.bin does not — never indexed), "empty" (no observations
// yet — nothing to index). An error means the log itself is unreadable.
func IndexState(root string) (string, error) {
	all, err := All(root)
	if err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "empty", nil
	}
	_, hdr, err := LoadIndex(root)
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "stale", nil
	}
	shaHex, err := cachedSourceSHA(root)
	if err != nil {
		return "", err
	}
	if hdr.SourceSHAHex() != shaHex {
		return "stale", nil
	}
	return "fresh", nil
}

// IndexStats reports the derived-artifact shape for tripwires
// (`memory doctor`, perf tests): format version, entry count and the
// pinned source sha. A retired v1 refuses with ErrHeaderMismatch —
// visible, not silent.
func IndexStats(root string) (version uint32, count int, sha string, err error) {
	_, hdr, err := LoadIndex(root)
	if err != nil {
		return 0, 0, "", err
	}
	return hdr.Version, hdr.Count, hdr.SourceSHAHex(), nil
}

// LoadIndex reads retrieval.bin, refusing untrusted headers with
// ErrHeaderMismatch. Vector bytes are data, never a trust decision.
func LoadIndex(root string) ([]IndexEntry, IndexHeader, error) {
	return LoadIndexWith(root, "")
}

// LoadIndexWith reads retrieval.bin expecting the named embedder (""
// means the default hash-ngram-v1). An unknown embedder id refuses; a
// header naming any other encoder refuses with ErrHeaderMismatch —
// rebuild with the same --embedder, never read foreign vectors. A
// retired v1 file refuses with ErrHeaderMismatch naming the repair,
// never "truncated": the version check runs before any record parse.
func LoadIndexWith(root, embedderID string) ([]IndexEntry, IndexHeader, error) {
	emb, err := LookupEmbedder(embedderID)
	if err != nil {
		return nil, IndexHeader{}, err
	}
	data, err := os.ReadFile(IndexPath(root))
	if err != nil {
		return nil, IndexHeader{}, err
	}
	hdr, off, err := parseHeader(data, emb.ID())
	if err != nil {
		return nil, IndexHeader{}, err
	}
	entries, err := parseRecords(data, off, hdr.Count)
	if err != nil {
		return nil, IndexHeader{}, err
	}
	return entries, hdr, nil
}

// parseHeader validates magic, version, embedder and dim against the
// expected encoder: a foreign embedder name is never trusted. A retired
// v1 header is refused with a friendly repair pointer, not a truncation
// error — the version gate runs before any record is touched.
func parseHeader(data []byte, wantEmbedder string) (IndexHeader, int, error) {
	var hdr IndexHeader
	off := 0
	magic, err := take(data, &off, 4)
	if err != nil || !bytes.Equal(magic, indexMagic[:]) {
		return hdr, 0, fmt.Errorf("%w: bad magic", ErrHeaderMismatch)
	}
	version, err := readU32(data, &off)
	if err != nil {
		return hdr, 0, err
	}
	if version == retiredIndexVersion {
		return hdr, 0, fmt.Errorf("%w: retired v1 index (offset table missing) — rebuild with `regulus memory reindex`", ErrHeaderMismatch)
	}
	if version != IndexVersion {
		return hdr, 0, fmt.Errorf("%w: version %d != %d — rebuild with `regulus memory reindex`", ErrHeaderMismatch, version, IndexVersion)
	}
	nameLen, err := readU16(data, &off)
	if err != nil {
		return hdr, 0, err
	}
	name, err := take(data, &off, int(nameLen))
	if err != nil {
		return hdr, 0, err
	}
	if string(name) != wantEmbedder {
		return hdr, 0, fmt.Errorf("%w: embedder %q != %q", ErrHeaderMismatch, name, wantEmbedder)
	}
	dim, err := readU32(data, &off)
	if err != nil {
		return hdr, 0, err
	}
	if int(dim) != EmbedDim {
		return hdr, 0, fmt.Errorf("%w: dim %d != %d", ErrHeaderMismatch, dim, EmbedDim)
	}
	count, err := readU32(data, &off)
	if err != nil {
		return hdr, 0, err
	}
	sha, err := take(data, &off, 32)
	if err != nil {
		return hdr, 0, err
	}
	hdr = IndexHeader{Version: version, Embedder: string(name), Dim: int(dim), Count: int(count)}
	copy(hdr.SourceSHA[:], sha)
	return hdr, off, nil
}

// parseRecords reads count ordered-index records from off.
func parseRecords(data []byte, off, count int) ([]IndexEntry, error) {
	entries := make([]IndexEntry, 0, count)
	for i := 0; i < count; i++ {
		var e IndexEntry
		var err error
		if e.ID, err = readSized(data, &off); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if e.Offset, err = readU64(data, &off); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		var ln uint32
		if ln, err = readU32(data, &off); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		e.Length = ln
		if e.TopicKey, err = readSized(data, &off); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if e.Domain, err = readSized(data, &off); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if e.Type, err = readSized(data, &off); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if e.Dimension, err = readSized(data, &off); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		vec, err := take(data, &off, EmbedDim)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		for j, b := range vec {
			e.Vec[j] = int8(b)
		}
		entries = append(entries, e)
	}
	if off != len(data) {
		return nil, fmt.Errorf("retrieval index has %d trailing byte(s)", len(data)-off)
	}
	return entries, nil
}

// readSized reads a u16-length-prefixed string at off.
func readSized(data []byte, off *int) (string, error) {
	n, err := readU16(data, off)
	if err != nil {
		return "", err
	}
	raw, err := take(data, off, int(n))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// take slices n bytes advancing off, or reports truncation.
func take(data []byte, off *int, n int) ([]byte, error) {
	if n < 0 || *off+n > len(data) {
		return nil, errors.New("retrieval index truncated")
	}
	out := data[*off : *off+n]
	*off += n
	return out, nil
}

// writeU16 appends a little-endian uint16.
func writeU16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

// writeU32 appends a little-endian uint32.
func writeU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

// writeU64 appends a little-endian uint64.
func writeU64(buf *bytes.Buffer, v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	buf.Write(b[:])
}

// readU16 reads a little-endian uint16 at off.
func readU16(data []byte, off *int) (uint16, error) {
	raw, err := take(data, off, 2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(raw), nil
}

// readU32 reads a little-endian uint32 at off.
func readU32(data []byte, off *int) (uint32, error) {
	raw, err := take(data, off, 4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(raw), nil
}

// readU64 reads a little-endian uint64 at off.
func readU64(data []byte, off *int) (uint64, error) {
	raw, err := take(data, off, 8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(raw), nil
}
