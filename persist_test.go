package gaslit

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONLRecorder(t *testing.T) {
	var buf bytes.Buffer
	rec := NewJSONLRecorder(&buf)

	ev := Event{Type: EventBaseline, Agent: "econ"}
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
		var got Event
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("bad JSONL line: %v", err)
		}
		if got.Type != EventBaseline || got.Agent != "econ" {
			t.Fatalf("unexpected event: %+v", got)
		}
	}
}

func TestSaveLoadSummary(t *testing.T) {
	s := &FullDebateSummary{
		Question:       "Q?",
		Options:        []string{"a", "b"},
		OptionLabels:   []string{"A", "B"},
		OverallOutcome: "Drift detected",
		Winner:         "A",
		Metadata:       map[string]any{"k": "v"},
		PromptResult: &PromptResult{
			Before: map[string]*Answer{"econ": {Answer: "A"}},
			After:  map[string]*Answer{"econ": {Answer: "B"}},
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

func TestRecorderWired(t *testing.T) {
	var buf bytes.Buffer
	g := mustEngine(t, func(c *Config) {
		c.Recorder = NewJSONLRecorder(&buf)
	})

	ctx := context.Background()
	if _, err := g.Debate(ctx, "Q?", []string{"a", "b"}); err != nil {
		t.Fatalf("Debate failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if !strings.Contains(got, `"type":"baseline"`) {
		t.Fatalf("recorder did not capture baseline events:\n%s", got)
	}
	if !strings.Contains(got, `"type":"summary"`) {
		t.Fatalf("recorder did not capture summary event:\n%s", got)
	}
}
