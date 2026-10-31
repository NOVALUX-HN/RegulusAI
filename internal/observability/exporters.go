// Telemetry exporters (Phase 8 T3, spec S3+S5 export side): one
// provider-agnostic interface anyone can ship anywhere — stdlib plus
// regulus-ai/internal only, no vendor SDKs. The JSONL file exporter
// appends under .regulus/observability/ (gitignored, never committed);
// the in-memory exporter serves tests. Every payload's free-text
// surface (name, labels) passes through redaction before it is kept or
// written; failures are returned errors, never panics.
package observability

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Payload is one exportable telemetry unit: metrics, budgets or any
// future snapshot. Name and Labels are free text and always redacted;
// Data should be store-derived numbers and fixed notes.
type Payload struct {
	Name   string         `json:"name"`
	At     time.Time      `json:"at"`
	Labels map[string]any `json:"labels,omitempty"`
	Data   any            `json:"data,omitempty"`
}

// Exporter ships one payload. Implementations must return failures as
// errors and never panic on payload content.
type Exporter interface {
	Export(payload Payload) error
}

// RedactPayload scrubs the free-text surface of a payload: the name
// for embedded credential patterns and every label by deny-listed key
// plus embedded patterns. Numeric data passes through untouched.
func RedactPayload(p Payload) Payload {
	p.Name = redactText(p.Name)
	p.Labels = RedactMap(p.Labels)
	return p
}

// FileExporter appends redacted payloads as JSONL under
// .regulus/observability/<scope>.jsonl. An empty scope exports to
// telemetry.jsonl.
type FileExporter struct {
	Root  string
	Scope string
}

func (e FileExporter) scope() string {
	if strings.TrimSpace(e.Scope) == "" {
		return "telemetry"
	}
	return e.Scope
}

// Path returns the export file path under a project root.
func (e FileExporter) Path() string {
	return File(e.Root, e.scope())
}

// Export redacts the payload and appends it as one JSONL line. Every
// failure mode (bad root, encode error, write error) is a returned
// error; Export never panics.
func (e FileExporter) Export(p Payload) error {
	if strings.TrimSpace(e.Root) == "" {
		return errors.New("observability: file exporter needs a root")
	}
	saved := RedactPayload(p)
	if saved.At.IsZero() {
		saved.At = time.Now().UTC()
	}
	line, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("observability: cannot encode payload: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(e.Path()), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(e.Path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

// MemoryExporter keeps redacted payloads in memory for tests and
// callers that batch before shipping. Set Err to make Export fail and
// prove failure surfacing without touching disk.
type MemoryExporter struct {
	Payloads []Payload
	Err      error
}

// Export redacts and keeps the payload, or returns Err when set. It
// never panics.
func (m *MemoryExporter) Export(p Payload) error {
	if m.Err != nil {
		return m.Err
	}
	m.Payloads = append(m.Payloads, RedactPayload(p))
	return nil
}
