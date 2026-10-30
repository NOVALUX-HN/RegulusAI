package almanac

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Retrieval index format (docs/memory-v2.md §5): header {magic,
// version, embedder, dim, count, sha256-of-source-JSONL} + count ×
// (id + dim × int8). Reindex is a pure function of
// almanac/observations.jsonl: twice on the same log → byte-identical
// file. A header mismatch (magic, version, embedder, dim) is never
// trusted — callers must rebuild.

// IndexMagic identifies a retrieval.bin file.
var indexMagic = [4]byte{'R', 'V', 'E', 'C'}

// IndexVersion is the current header version.
const IndexVersion uint32 = 1

// ErrHeaderMismatch signals an untrusted index: magic, version,
// embedder or dim differs — rebuild, never read.
var ErrHeaderMismatch = errors.New("almanac: retrieval index header mismatch — rebuild with `regulus memory reindex`")

// ErrNoLog signals there is no observation log to index yet.
var ErrNoLog = errors.New("almanac: no observations log yet — save an observation first")

// IndexEntry is one indexed observation.
type IndexEntry struct {
	ID  string
	Vec [EmbedDim]int8
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
func ReindexWith(root, embedderID string) (int, string, error) {
	emb, err := LookupEmbedder(embedderID)
	if err != nil {
		return 0, "", err
	}
	raw, err := os.ReadFile(File(root))
	if os.IsNotExist(err) {
		return 0, "", ErrNoLog
	}
	if err != nil {
		return 0, "", err
	}
	all, err := All(root)
	if err != nil {
		return 0, "", err
	}
	if len(all) == 0 {
		return 0, "", ErrNoLog
	}
	sum := sha256.Sum256(raw)
	buf := newIndexBuffer(emb.ID(), len(all), sum)
	for _, o := range all {
		writeEntry(buf, o, emb)
	}
	if err := os.MkdirAll(filepath.Dir(IndexPath(root)), 0o755); err != nil {
		return 0, "", err
	}
	if err := os.WriteFile(IndexPath(root), buf.Bytes(), 0o644); err != nil {
		return 0, "", err
	}
	return len(all), hex.EncodeToString(sum[:]), nil
}

// newIndexBuffer writes the fixed header prefix.
func newIndexBuffer(embedder string, count int, sum [32]byte) *bytes.Buffer {
	buf := &bytes.Buffer{}
	buf.Write(indexMagic[:])
	writeU32(buf, IndexVersion)
	writeU16(buf, uint16(len(embedder)))
	buf.WriteString(embedder)
	writeU32(buf, uint32(EmbedDim))
	writeU32(buf, uint32(count))
	buf.Write(sum[:])
	return buf
}

// writeEntry appends one (id + vector) record.
func writeEntry(buf *bytes.Buffer, o Observation, emb Embedder) {
	writeU16(buf, uint16(len(o.ID)))
	buf.WriteString(o.ID)
	vec := emb.EmbedObservation(o)
	for _, b := range vec {
		buf.WriteByte(byte(b))
	}
}

// IndexState reports the retrieval index freshness for status surfaces:
// "fresh" (hybrid ranking active), "stale" (the log changed since the
// last reindex, or the index is corrupt — search degrades to lexical),
// "missing" (observations exist but retrieval.bin does not — never
// indexed), "empty" (no observations yet — nothing to index). An error
// means the log itself is unreadable. Stale, missing and corrupt share
// one repair: `regulus memory reindex`.
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
	raw, err := os.ReadFile(File(root))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	if hdr.SourceSHAHex() != hex.EncodeToString(sum[:]) {
		return "stale", nil
	}
	return "fresh", nil
}

// LoadIndex reads retrieval.bin, refusing untrusted headers with
// ErrHeaderMismatch. Vector bytes are data, never a trust decision.
func LoadIndex(root string) ([]IndexEntry, IndexHeader, error) {
	return LoadIndexWith(root, "")
}

// LoadIndexWith reads retrieval.bin expecting the named embedder (""
// means the default hash-ngram-v1). An unknown embedder id refuses; a
// header naming any other encoder refuses with ErrHeaderMismatch —
// rebuild with the same --embedder, never read foreign vectors.
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
	entries, err := parseEntries(data, off, hdr.Count)
	if err != nil {
		return nil, IndexHeader{}, err
	}
	return entries, hdr, nil
}

// parseHeader validates magic, version, embedder and dim against the
// expected encoder: a foreign embedder name is never trusted.
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
	if version != IndexVersion {
		return hdr, 0, fmt.Errorf("%w: version %d != %d", ErrHeaderMismatch, version, IndexVersion)
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

// parseEntries reads count (id + vector) records from off.
func parseEntries(data []byte, off, count int) ([]IndexEntry, error) {
	entries := make([]IndexEntry, 0, count)
	for i := 0; i < count; i++ {
		idLen, err := readU16(data, &off)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		id, err := take(data, &off, int(idLen))
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		vec, err := take(data, &off, EmbedDim)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		var e IndexEntry
		e.ID = string(id)
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
