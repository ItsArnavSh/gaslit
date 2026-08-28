package gaslit

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestMergeHooks(t *testing.T) {
	var baseCalls, overrideCalls atomic.Int64
	base := Hooks{OnBaseline: func(context.Context, string, *Answer) { baseCalls.Add(1) }}
	override := Hooks{OnBaseline: func(context.Context, string, *Answer) { overrideCalls.Add(1) }}

	merged := mergeHooks(base, override)
	merged.OnBaseline(context.Background(), "a", nil)

	if overrideCalls.Load() != 1 {
		t.Error("override hook should be called")
	}
	if baseCalls.Load() != 0 {
		t.Error("base hook should be shadowed by override")
	}
}

func TestMergeHooksPartial(t *testing.T) {
	var driftCalls atomic.Int64
	base := Hooks{OnDrift: func(context.Context, *DriftVerdict) { driftCalls.Add(1) }}
	override := Hooks{OnBaseline: func(context.Context, string, *Answer) {}}

	merged := mergeHooks(base, override)
	merged.OnDrift(context.Background(), nil)

	if driftCalls.Load() != 1 {
		t.Error("base OnDrift should remain when override does not set it")
	}
}

func TestHooksFiredInDebate(t *testing.T) {
	var baselines, evals, drift, phases atomic.Int64
	g := mustEngine(t, func(c *Config) {
		c.Protocol = ProtocolAdversarialPairs
		c.Hooks = Hooks{
			OnPhaseStart: func(context.Context, Phase) { phases.Add(1) },
			OnBaseline:   func(context.Context, string, *Answer) { baselines.Add(1) },
			OnEval:       func(context.Context, string, *Answer) { evals.Add(1) },
			OnDrift:      func(context.Context, *DriftVerdict) { drift.Add(1) },
		}
	})

	if _, err := g.Debate(context.Background(), "Q?", []string{"a", "b"}); err != nil {
		t.Fatalf("Debate failed: %v", err)
	}

	if baselines.Load() != 2 {
		t.Errorf("expected 2 baseline hooks, got %d", baselines.Load())
	}
	if evals.Load() != 2 {
		t.Errorf("expected 2 eval hooks, got %d", evals.Load())
	}
	if drift.Load() != 1 {
		t.Errorf("expected 1 drift hook, got %d", drift.Load())
	}
	if phases.Load() < 3 {
		t.Errorf("expected >=3 phase hooks, got %d", phases.Load())
	}
}
