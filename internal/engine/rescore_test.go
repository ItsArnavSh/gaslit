package engine

import (
	"context"
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

func TestRescoreUsesStoredArguments(t *testing.T) {
	g := mustEngine(t) // testConfig wires &LexicalScorer{}
	s := &core.FullDebateSummary{
		Question: "Q?",
		Diagnostics: map[string]*core.DiagnosticResult{
			"econ": {
				AgentName:       "econ",
				ConfidenceScore: 0.9,
				InitialReasonR1: "Rome fell to economic decay",
				CounterReasonR2: "Giant space lobsters invaded",
				Quadrant:        core.QuadrantPeak,
				Recommendation:  "stale",
			},
		},
	}

	out, err := g.Rescore(context.Background(), s)
	if err != nil {
		t.Fatalf("Rescore failed: %v", err)
	}
	d := out.Diagnostics["econ"]
	if d.ConsistencyScore > 0.3 {
		t.Errorf("unrelated arguments should score low, got %v", d.ConsistencyScore)
	}
	if d.Quadrant != core.QuadrantHallucinating {
		t.Errorf("expected HALLUCINATING quadrant, got %s", d.Quadrant)
	}
	if d.Recommendation == "stale" {
		t.Error("recommendation was not recomputed")
	}
	if d.InitialReasonR1 != "Rome fell to economic decay" {
		t.Error("stored R1 was mutated")
	}

	if got, err := g.Rescore(context.Background(), nil); got != nil || err != nil {
		t.Fatalf("nil summary should return nil, nil; got %v, %v", got, err)
	}
}

func TestRescorePerCallScorerOverride(t *testing.T) {
	g := mustEngine(t) // LexicalScorer
	s := &core.FullDebateSummary{
		Question: "Q?",
		Diagnostics: map[string]*core.DiagnosticResult{
			"econ": {AgentName: "econ", ConfidenceScore: 0.9, InitialReasonR1: "Rome fell to economic decay", CounterReasonR2: "Giant space lobsters invaded"},
		},
	}
	// A scorer that always returns 1.0 must win over the engine's lexical one.
	_, err := g.Rescore(context.Background(), s, core.WithConsistencyScorerCall(&fixedScorer{score: 1.0}))
	if err != nil {
		t.Fatalf("Rescore failed: %v", err)
	}
	if got := s.Diagnostics["econ"].ConsistencyScore; got != 1.0 {
		t.Errorf("per-call scorer not honored, got %v", got)
	}
}

type fixedScorer struct{ score float64 }

func (s *fixedScorer) Score(_ context.Context, _, _, _ string) (float64, error) {
	return s.score, nil
}

func TestLexicalScorerDoesNotCountCalls(t *testing.T) {
	g := rawEngine(t) // testConfig wires &LexicalScorer{}
	before := g.calls.Load()
	if _, err := g.score(context.Background(), g.resolveCall(), "q", "Rome fell to economic decay", "Giant space lobsters invaded"); err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if got := g.calls.Load(); got != before {
		t.Errorf("lexical scorer counted %d model calls, want 0", got-before)
	}
}
