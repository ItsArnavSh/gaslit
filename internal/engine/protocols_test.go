package engine

import (
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

func TestPlanChallenges(t *testing.T) {
	names := []string{"econ", "pol", "cult"}
	before := map[string]*core.Answer{
		"econ": {Answer: "A", Confidences: map[string]float64{"A": 0.9, "B": 0.05, "C": 0.05}, Reasoning: "trade"},
		"pol":  {Answer: "B", Confidences: map[string]float64{"A": 0.1, "B": 0.8, "C": 0.1}, Reasoning: "war"},
		"cult": {Answer: "C", Confidences: map[string]float64{"A": 0.1, "B": 0.1, "C": 0.8}, Reasoning: "ideas"},
	}
	cp := func(from, claim, reasoning string) string { return "challenge" }

	t.Run("round robin", func(t *testing.T) {
		challenges := planChallenges(core.ProtocolRoundRobin, names, before, cp)
		// 3 agents -> 3*2 = 6 ordered challenges
		if len(challenges) != 6 {
			t.Fatalf("expected 6 challenges, got %d", len(challenges))
		}
		for _, ch := range challenges {
			if ch.Target == ch.From {
				t.Fatalf("self challenge: %+v", ch)
			}
		}
	})

	t.Run("adversarial pairs", func(t *testing.T) {
		// 3 agents -> one pair (econ,pol) + leftover (cult, econ) = 2 pairs = 4 challenges
		challenges := planChallenges(core.ProtocolAdversarialPairs, names, before, cp)
		if len(challenges) != 4 {
			t.Fatalf("expected 4 challenges, got %d", len(challenges))
		}
	})

	t.Run("devils advocate", func(t *testing.T) {
		challenges := planChallenges(core.ProtocolDevilsAdvocate, names, before, cp)
		if len(challenges) != 3 {
			t.Fatalf("expected 3 challenges, got %d", len(challenges))
		}
	})

	t.Run("supervised yields none", func(t *testing.T) {
		challenges := planChallenges(core.ProtocolSupervised, names, before, cp)
		if len(challenges) != 0 {
			t.Fatalf("expected no directed challenges, got %d", len(challenges))
		}
	})
}

func TestPlanChallengesAllAgree(t *testing.T) {
	names := []string{"econ", "pol"}
	before := map[string]*core.Answer{
		"econ": {Answer: "A", Confidences: map[string]float64{"A": 0.9, "B": 0.1}},
		"pol":  {Answer: "A", Confidences: map[string]float64{"A": 0.7, "B": 0.3}},
	}
	cp := func(from, claim, reasoning string) string { return "challenge" }

	challenges := planChallenges(core.ProtocolDevilsAdvocate, names, before, cp)
	if len(challenges) != 2 {
		t.Fatalf("expected 2 challenges, got %d", len(challenges))
	}
	// Both targets should receive a local devil's advocate prompt.
	for _, ch := range challenges {
		if ch.From != "devil's advocate" {
			t.Fatalf("expected local devil's advocate, got from=%q", ch.From)
		}
	}
}

func TestDevilsAdvocateLocal(t *testing.T) {
	ans := &core.Answer{Confidences: map[string]float64{"A": 0.9, "B": 0.05, "C": 0.05}}
	p := devilsAdvocateLocal(ans)
	if p == "" {
		t.Fatal("expected non-empty local challenge")
	}
}
