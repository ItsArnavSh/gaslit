package gaslit

import (
	"strings"
	"testing"
)

func TestExtractJSONObject(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"clean json", `{"a":1}`, true},
		{"leading prose", `Sure: {"a":1}`, true},
		{"trailing prose", `{"a":1} done`, true},
		{"markdown fence", "```json\n{\"a\":1}\n```", true},
		{"nested braces", `{"a":{"b":[1,2]}}`, true},
		{"no closing brace", `{"a":1`, false},
		{"no object", `lorem ipsum`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSONObject(tt.raw)
			if tt.ok {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.HasPrefix(strings.TrimSpace(got), "{") || !strings.HasSuffix(strings.TrimSpace(got), "}") {
					t.Fatalf("extracted %q not an object", got)
				}
			} else if err == nil {
				t.Fatalf("expected error, got %q", got)
			}
		})
	}
}

func TestMCQSchemaParse(t *testing.T) {
	s := &MCQSchema{}
	valid := `{"answer":"A","confidences":{"A":0.9,"B":0.1},"reasoning":"because"}`
	a, err := s.Parse("Here you go:\n" + valid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Answer != "A" || a.Confidences["B"] != 0.1 {
		t.Fatalf("unexpected answer: %+v", a)
	}

	if _, err := s.Parse(`{"answer":"A","confidences":{},"reasoning":"r"}`); err == nil {
		t.Fatal("expected error for empty confidences in MCQ schema")
	}

	withChallenge := `{"answer":"B","confidences":{"A":0.2,"B":0.8},"reasoning":"trade","challenge":{"agent_name":"pol","prompt":"defend X"}}`
	ca, err := s.Parse(withChallenge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ca.Challenge == nil || ca.Challenge.AgentName != "pol" {
		t.Fatalf("challenge not parsed: %+v", ca.Challenge)
	}

	if !strings.Contains(s.Instruction(), `"confidences"`) {
		t.Error("MCQ instruction should mention confidences")
	}
}

func TestOpenEndedSchemaParse(t *testing.T) {
	s := &OpenEndedSchema{}
	a, err := s.Parse(`{"answer":"The economy collapsed due to hyperinflation.","reasoning":"inflation eroded the tax base"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Answer == "" || a.Confidences != nil {
		t.Fatalf("unexpected answer: %+v", a)
	}

	if _, err := s.Parse(`{"answer":"","reasoning":"r"}`); err == nil {
		t.Fatal("expected error for empty answer")
	}
	if strings.Contains(s.Instruction(), `"confidences"`) {
		t.Error("open-ended instruction should not mention confidences")
	}
}

func TestSchemaFor(t *testing.T) {
	g := rawEngine(t)
	if _, ok := g.schemaFor([]string{"a"}).(*MCQSchema); !ok {
		t.Error("expected MCQ schema for optioned question")
	}
	if _, ok := g.schemaFor(nil).(*OpenEndedSchema); !ok {
		t.Error("expected open-ended schema for option-less question")
	}
}
