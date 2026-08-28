package gaslit

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
)

func TestStreamEvents(t *testing.T) {
	g := mustEngine(t, func(c *Config) { c.Protocol = ProtocolAdversarialPairs })

	ctx := context.Background()
	ch, err := g.Stream(ctx, "Q?", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var sawBaseline, sawEval, sawDone, sawErr bool
	for ev := range ch {
		switch ev.Type {
		case EventBaseline:
			sawBaseline = true
		case EventEval:
			sawEval = true
		case EventDone:
			sawDone = true
		case EventError:
			sawErr = true
		}
	}

	if !sawBaseline {
		t.Error("expected baseline stream events")
	}
	if !sawEval {
		t.Error("expected eval stream events")
	}
	if !sawDone {
		t.Error("expected done event")
	}
	if sawErr {
		t.Error("unexpected error event")
	}
}

func TestBudgetCallLimit(t *testing.T) {
	g := mustEngine(t, func(c *Config) {
		c.Protocol = ProtocolAdversarialPairs
		c.Budget = CostBudget{MaxCalls: 1}
	})

	ctx := context.Background()
	if _, err := g.Debate(ctx, "Q?", []string{"a", "b"}); err == nil {
		t.Fatal("expected budget error after 1 call")
	} else if !strings.Contains(err.Error(), "budget exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBudgetResetPerDebateCallOption(t *testing.T) {
	g := mustEngine(t, func(c *Config) { c.Protocol = ProtocolAdversarialPairs })

	ctx := context.Background()
	// unlimited by default succeeds
	if _, err := g.Debate(ctx, "Q?", []string{"a", "b"}); err != nil {
		t.Fatalf("Debate failed: %v", err)
	}
	// a per-call budget that is too small fails even though the engine is unlimited
	if _, err := g.Debate(ctx, "Q?", []string{"a", "b"}, WithBudgetCall(CostBudget{MaxCalls: 1})); err == nil {
		t.Fatal("expected budget error for per-call budget of 1 call")
	}
}

func TestRedaction(t *testing.T) {
	// Baseline prompt contains the question text; ensure the fake agent receives
	// the redacted version by inspecting the reply content path via a spy agent.
	var received string
	spy := &fakeAgent{
		name: "spy",
		reply: func(last string) string {
			received = last
			return `{"answer":"A","confidences":{"A":0.9,"B":0.1},"reasoning":"r"}`
		},
	}
	cfg := testConfig(func(c *Config) {
		c.Protocol = ProtocolAdversarialPairs
		c.Redact = func(s string) string { return strings.ReplaceAll(s, "SECRET", "[REDACTED]") }
		c.Agents = []adk.Agent{spy, &fakeAgent{name: "other"}}
	})
	g2, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	if _, err := g2.Baseline(ctx, "What is the SECRET plan?", []string{"a", "b"}); err != nil {
		t.Fatalf("Baseline failed: %v", err)
	}
	if strings.Contains(received, "SECRET") {
		t.Errorf("prompt was not redacted: %q", received)
	}
	if !strings.Contains(received, "[REDACTED]") {
		t.Errorf("redaction marker missing from prompt: %q", received)
	}
}
