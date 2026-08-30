package aggregation

import (
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

func TestAggregation(t *testing.T) {
	labels := []string{"A", "B", "C"}
	after := map[string]*core.Answer{
		"econ": {Answer: "A", Confidences: map[string]float64{"A": 0.9, "B": 0.05, "C": 0.05}},
		"pol":  {Answer: "A", Confidences: map[string]float64{"A": 0.6, "B": 0.3, "C": 0.1}},
		"cult": {Answer: "B", Confidences: map[string]float64{"A": 0.1, "B": 0.8, "C": 0.1}},
	}

	if got := MajorityVote(after, labels, nil); got != "A" {
		t.Errorf("MajorityVote = %q, want A", got)
	}
	if got := ConfidenceWeightedVote(after, labels, nil); got != "A" {
		t.Errorf("ConfidenceWeightedVote = %q, want A", got)
	}

	weights := map[string]float64{"cult": 10.0}
	if got := WeightedVote(after, labels, weights); got != "B" {
		t.Errorf("WeightedVote with cult weight = %q, want B", got)
	}
}

func TestAggregationTie(t *testing.T) {
	after := map[string]*core.Answer{
		"econ": {Answer: "A"},
		"pol":  {Answer: "B"},
	}
	// Tie (1 vs 1) -> no winner
	if got := MajorityVote(after, nil, nil); got != "" {
		t.Errorf("tie should yield no winner, got %q", got)
	}
}

func TestTopOptionFloat(t *testing.T) {
	if got := topOptionFloat(map[string]float64{}); got != "" {
		t.Errorf("empty votes should yield no winner, got %q", got)
	}
}
