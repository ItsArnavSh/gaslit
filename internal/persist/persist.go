// Package persist handles event recording and the save/load round-trip for
// debate summaries.
package persist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

// JSONLRecorder writes one JSON Event per line to an io.Writer.
type JSONLRecorder struct {
	mu sync.Mutex
	w  io.Writer
}

// NewJSONLRecorder returns a JSONLRecorder writing to w.
func NewJSONLRecorder(w io.Writer) *JSONLRecorder {
	return &JSONLRecorder{w: w}
}

// Record marshals and writes ev as a single JSON line.
func (r *JSONLRecorder) Record(_ context.Context, ev core.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = r.w.Write(append(raw, '\n'))
	return err
}

// SaveSummary writes s to w as indented JSON.
func SaveSummary(w io.Writer, s *core.FullDebateSummary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

// LoadSummary reads a FullDebateSummary from r.
func LoadSummary(r io.Reader) (*core.FullDebateSummary, error) {
	var s core.FullDebateSummary
	dec := json.NewDecoder(r)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("failed to decode summary: %w", err)
	}
	return &s, nil
}

// SaveSummaryFile writes s as indented JSON to the file at path. Useful for
// caching a debate run so scoring/quadrant logic can be re-run later via
// LoadSummaryFile + ReclassifySummary/Rescore without new model calls.
func SaveSummaryFile(path string, s *core.FullDebateSummary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return SaveSummary(f, s)
}

// LoadSummaryFile reads a FullDebateSummary from the file at path.
func LoadSummaryFile(path string) (*core.FullDebateSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return LoadSummary(f)
}
