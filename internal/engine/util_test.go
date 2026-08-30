package engine

import (
	"context"
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/cloudwego/eino/schema"
)

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

func TestAgentInstruction(t *testing.T) {
	empty := &agentState{}
	if agentInstruction(empty) != "" {
		t.Fatal("expected empty instruction for default agent")
	}
	withCfg := &agentState{cfg: core.AgentConfig{Instruction: "You are a historian."}}
	if got := agentInstruction(withCfg); got != "You are a historian.\n\n" {
		t.Fatalf("unexpected instruction: %q", got)
	}
}

func TestQueryAgentRetry(t *testing.T) {
	// First reply is garbage; second reply is valid JSON.
	var calls int
	agent := &fakeAgent{
		name: "flaky",
		reply: func(string) string {
			calls++
			if calls == 1 {
				return "not json at all"
			}
			return `{"answer":"A","confidences":{"A":0.9,"B":0.1},"reasoning":"ok"}`
		},
	}
	g := rawEngine(t)
	g.states["flaky"] = &agentState{agent: agent, history: make([]*schema.Message, 0)}

	ans, err := g.queryAgent(t.Context(), "flaky", "prompt", true, &core.MCQSchema{})
	if err != nil {
		t.Fatalf("queryAgent failed: %v", err)
	}
	if ans.Answer != "A" {
		t.Fatalf("unexpected answer: %+v", ans)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestQueryAgentGivesUpAfterRetries(t *testing.T) {
	agent := &fakeAgent{name: "broken", reply: func(string) string { return "still not json" }}
	g := rawEngine(t, func(c *core.Config) { c.MaxRetries = 1 })
	g.states["broken"] = &agentState{agent: agent, history: make([]*schema.Message, 0)}

	if _, err := g.queryAgent(t.Context(), "broken", "prompt", true, &core.MCQSchema{}); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestStateSkipped(t *testing.T) {
	g := rawEngine(t)
	if g.stateSkipped("econ", core.PhaseBaseline) {
		t.Fatal("agent should not be skipped by default")
	}
	g.states["econ"].cfg.SkipPhases = map[core.Phase]bool{core.PhaseBaseline: true}
	if !g.stateSkipped("econ", core.PhaseBaseline) {
		t.Fatal("agent should be skipped when configured")
	}
	if g.stateSkipped("econ", core.PhaseEvaluation) {
		t.Fatal("agent should not be skipped for other phases")
	}
}

func TestForEachAgentSequential(t *testing.T) {
	g := rawEngine(t)
	var order []string
	err := g.forEachAgent(t.Context(), g.agentNames, func(_ context.Context, name string) error {
		order = append(order, name)
		return nil
	})
	if err != nil {
		t.Fatalf("forEachAgent failed: %v", err)
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(order))
	}
	if order[0] != "econ" || order[1] != "pol" {
		t.Fatalf("unexpected order: %v", order)
	}
}
