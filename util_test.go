package gaslit

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestParseJSON(t *testing.T) {
	valid := `{"answer":"A","confidences":{"A":0.9,"B":0.1},"reasoning":"because"}`
	validConf := map[string]float64{"A": 0.9, "B": 0.1}

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"clean json", valid, false},
		{"leading prose stripped", "Sure! Here you go:\n" + valid, false},
		{"trailing prose stripped", valid + "\nHope this helps!", false},
		{"markdown fenced", "```json\n" + valid + "\n```", false},
		{"nested braces in reasoning", `{"answer":"A","confidences":{"A":0.9,"B":0.1},"reasoning":"see {diagram}"}`, false},
		{"empty confidences", `{"answer":"A","confidences":{},"reasoning":"r"}`, true},
		{"no closing brace", `{"answer":"A","confidences":{"A":0.9}`, true},
		{"not json", "lorem ipsum", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseJSON(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Answer != "A" {
				t.Fatalf("expected answer A, got %q", got.Answer)
			}
			if len(got.Confidences) != len(validConf) {
				t.Fatalf("expected %d confidences, got %d", len(validConf), len(got.Confidences))
			}
		})
	}
}

func TestParseJSONNestedObjectChallenge(t *testing.T) {
	raw := `{"answer":"B","confidences":{"A":0.2,"B":0.8},"reasoning":"trade","challenge":{"agent_name":"pol","prompt":"defend X"}}`
	got, err := parseJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Challenge == nil {
		t.Fatal("expected challenge to be parsed")
	}
	if got.Challenge.AgentName != "pol" {
		t.Fatalf("expected challenge agent pol, got %q", got.Challenge.AgentName)
	}
}

func TestSanitizeHistoryMessage(t *testing.T) {
	if got := sanitizeHistoryMessage(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
	raw := &schema.Message{Content: "hi"}
	sanitized := sanitizeHistoryMessage(raw)
	if sanitized == nil {
		t.Fatal("expected non-nil result")
	}
	if sanitized.Content != "hi" {
		t.Fatalf("expected content hi, got %q", sanitized.Content)
	}
}

func TestBuildInitialPrompt(t *testing.T) {
	labels, prompt := (&Gaslit{}).buildInitialPrompt("Q?", []string{"one", "two", "three"})
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(labels))
	}
	if strings.Join(labels, "") != "ABC" {
		t.Fatalf("expected labels ABC, got %v", labels)
	}
	for _, want := range []string{"Q?", "A) one", "B) two", "C) three", `"answer"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestClassifyQuadrant(t *testing.T) {
	tests := []struct {
		name    string
		conf    float64
		consist float64
		want    QuadrantState
	}{
		{"peak", 0.9, 0.9, QuadrantPeak},
		{"hallucinating", 0.9, 0.3, QuadrantHallucinating},
		{"kb deficit", 0.3, 0.9, QuadrantKBDeficit},
		{"wrong", 0.3, 0.3, QuadrantWrong},
		{"boundary confidence high", 0.55, 0.61, QuadrantPeak},
		{"boundary confidence low", 0.54, 0.61, QuadrantKBDeficit},
		{"boundary consistency high", 0.61, 0.60, QuadrantPeak},
		{"boundary consistency low", 0.61, 0.59, QuadrantHallucinating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quadrant, rec := classifyQuadrant(tt.conf, tt.consist)
			if quadrant != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, quadrant)
			}
			if rec == "" {
				t.Fatal("expected non-empty recommendation")
			}
		})
	}
}
