package gaslit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// EventType identifies a recorded pipeline event.
type EventType string

const (
	EventPhaseStart EventType = "phase_start"
	EventPhaseEnd   EventType = "phase_end"
	EventBaseline   EventType = "baseline"
	EventCrossExam  EventType = "cross_exam"
	EventEval       EventType = "evaluation"
	EventDrift      EventType = "drift"
	EventDiagnostic EventType = "diagnostic"
	EventSummary    EventType = "summary"
	EventDone       EventType = "done"
	EventError      EventType = "error"
)

// Event is a JSON-serializable record of a pipeline milestone, suitable for
// persistence or replay.
type Event struct {
	Time  time.Time       `json:"time"`
	Type  EventType       `json:"type"`
	Agent string          `json:"agent,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Recorder receives an Event stream for persistence.
type Recorder interface {
	Record(ctx context.Context, ev Event) error
}

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
func (r *JSONLRecorder) Record(_ context.Context, ev Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = r.w.Write(append(raw, '\n'))
	return err
}

// record marshals data and sends it to the configured recorder, if any.
func (g *Gaslit) record(evType EventType, agent string, data any) {
	if g.cfg.Recorder == nil {
		return
	}
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		raw = b
	}
	ev := Event{Time: time.Now().UTC(), Type: evType, Agent: agent, Data: raw}
	_ = g.cfg.Recorder.Record(context.Background(), ev)
}

// SaveSummary writes s to w as indented JSON.
func SaveSummary(w io.Writer, s *FullDebateSummary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

// LoadSummary reads a FullDebateSummary from r.
func LoadSummary(r io.Reader) (*FullDebateSummary, error) {
	var s FullDebateSummary
	dec := json.NewDecoder(r)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("failed to decode summary: %w", err)
	}
	return &s, nil
}
