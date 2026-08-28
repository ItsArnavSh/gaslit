package gaslit

import (
	"context"
	"testing"
)

func TestClassifyWith(t *testing.T) {
	thr := Thresholds{DriftSignificance: 0.15, Confidence: 0.55, Consistency: 0.60}
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
			quadrant, rec := classifyWith(tt.conf, tt.consist, thr)
			if quadrant != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, quadrant)
			}
			if rec == "" {
				t.Fatal("expected non-empty recommendation")
			}
		})
	}
}

func TestThresholdClassifier(t *testing.T) {
	// ThresholdClassifier uses built-in defaults.
	q, _ := ThresholdClassifier(0.9, 0.9)
	if q != QuadrantPeak {
		t.Fatalf("expected PEAK, got %s", q)
	}
}

func TestLexicalScorer(t *testing.T) {
	s := &LexicalScorer{}

	score, err := s.Score(context.Background(), "q", "The economy collapsed due to inflation", "The economy collapsed due to inflation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0.9 {
		t.Errorf("identical arguments should score high, got %v", score)
	}

	score, err = s.Score(context.Background(), "q", "Rome fell to economic decay", "Giant space lobsters invaded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score > 0.3 {
		t.Errorf("unrelated arguments should score low, got %v", score)
	}

	score, err = s.Score(context.Background(), "q", "", "")
	if err != nil || score != 0.5 {
		t.Errorf("empty arguments should score 0.5, got %v err %v", score, err)
	}
}
