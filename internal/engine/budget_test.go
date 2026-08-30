package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/cloudwego/eino/adk"
)

func TestBudgetCapsDebateCalls(t *testing.T) {
	g := mustEngine(t, func(c *core.Config) {
		c.Budget = core.CostBudget{MaxCalls: 1}
	})

	_, err := g.Debate(context.Background(), "Q?", []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected budget error")
	}
	var berr *core.BudgetError
	if !errors.As(err, &berr) {
		t.Fatalf("expected *BudgetError, got %T: %v", err, err)
	}
}

func TestBudgetStopsRunawayRetries(t *testing.T) {
	// An agent that never returns parseable JSON would otherwise spin through
	// MaxRetries (or forever on a buggy retry loop); the budget must trip first.
	broken := &fakeAgent{name: "broken", reply: func(string) string { return "not json" }}

	cfg := testConfig(func(c *core.Config) {
		c.MaxRetries = 2
		c.Budget = core.CostBudget{MaxCalls: 2}
	})
	cfg.Agents = []adk.Agent{broken}

	g, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	_, err = g.Debate(context.Background(), "Q?", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected an error")
	}
	var berr *core.BudgetError
	if !errors.As(err, &berr) {
		t.Fatalf("expected *BudgetError from the retry loop, got %T: %v", err, err)
	}
}
