package engine

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/persist"
)

func TestHooksFiredInDebate(t *testing.T) {
	var baselines, evals, drift, phases atomic.Int64
	g := mustEngine(t, func(c *core.Config) {
		c.Protocol = core.ProtocolAdversarialPairs
		c.Hooks = core.Hooks{
			OnPhaseStart: func(context.Context, core.Phase) { phases.Add(1) },
			OnBaseline:   func(context.Context, string, *core.Answer) { baselines.Add(1) },
			OnEval:       func(context.Context, string, *core.Answer) { evals.Add(1) },
			OnDrift:      func(context.Context, *core.DriftVerdict) { drift.Add(1) },
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

func TestRecorderWired(t *testing.T) {
	var buf strings.Builder
	rec := persist.NewJSONLRecorder(&buf)
	g := mustEngine(t, func(c *core.Config) {
		c.Recorder = rec
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
