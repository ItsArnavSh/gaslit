package persist

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

func TestJSONLRecorder(t *testing.T) {
	var buf bytes.Buffer
	rec := NewJSONLRecorder(&buf)

	ev := core.Event{Type: core.EventBaseline, Agent: "econ"}
	if err := rec.Record(context.Background(), ev); err != nil {
		t.Fatalf("record failed: %v", err)
	}
	if err := rec.Record(context.Background(), ev); err != nil {
		t.Fatalf("record failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, line := range lines {
		var got core.Event
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("bad JSONL line: %v", err)
		}
		if got.Type != core.EventBaseline || got.Agent != "econ" {
			t.Fatalf("unexpected event: %+v", got)
		}
	}
}

func TestSaveLoadSummary(t *testing.T) {
	s := &core.FullDebateSummary{
		Question:       "Q?",
		Options:        []string{"a", "b"},
		OptionLabels:   []string{"A", "B"},
		OverallOutcome: "Drift detected",
		Winner:         "A",
		Metadata:       map[string]any{"k": "v"},
		PromptResult: &core.PromptResult{
			Before: map[string]*core.Answer{"econ": {Answer: "A"}},
			After:  map[string]*core.Answer{"econ": {Answer: "B"}},
		},
	}

	var buf bytes.Buffer
	if err := SaveSummary(&buf, s); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := LoadSummary(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got.Question != "Q?" || got.Winner != "A" || got.Metadata["k"] != "v" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.PromptResult.Before["econ"].Answer != "A" {
		t.Fatalf("prompt result not roundtripped: %+v", got.PromptResult)
	}
}

func TestSaveLoadSummaryFile(t *testing.T) {
	path := t.TempDir() + "/summary.json"
	s := &core.FullDebateSummary{
		Question:       "Q?",
		Winner:         "A",
		OverallOutcome: "Drift detected",
		Diagnostics: map[string]*core.DiagnosticResult{
			"econ": {AgentName: "econ", ConfidenceScore: 0.9, ConsistencyScore: 0.2, Quadrant: core.QuadrantHallucinating},
		},
	}

	if err := SaveSummaryFile(path, s); err != nil {
		t.Fatalf("save file failed: %v", err)
	}

	got, err := LoadSummaryFile(path)
	if err != nil {
		t.Fatalf("load file failed: %v", err)
	}
	if got.Question != "Q?" || got.Winner != "A" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if d := got.Diagnostics["econ"]; d == nil || d.Quadrant != core.QuadrantHallucinating {
		t.Fatalf("diagnostics not roundtripped: %+v", got.Diagnostics)
	}
}
