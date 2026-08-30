package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/cloudwego/eino/adk"
)

func TestStatsAfterDebate(t *testing.T) {
	g := mustEngine(t, func(c *core.Config) { c.Protocol = core.ProtocolAdversarialPairs })

	if _, err := g.Debate(context.Background(), "Q?", []string{"a", "b"}); err != nil {
		t.Fatalf("Debate failed: %v", err)
	}

	stats := g.Stats()
	if stats.TotalCalls == 0 {
		t.Error("expected non-zero total call count")
	}
	if len(stats.AgentCalls) == 0 {
		t.Error("expected per-agent call counts")
	}
	if len(stats.PhaseDuration) == 0 {
		t.Error("expected phase durations to be recorded")
	}
	if stats.Tokens == 0 {
		t.Error("expected token estimate")
	}
}

func TestDebateMany(t *testing.T) {
	g := mustEngine(t, func(c *core.Config) { c.Protocol = core.ProtocolAdversarialPairs })
	ctx := context.Background()

	summaries, err := g.DebateMany(ctx,
		[]string{"Q1?", "Q2?"},
		[][]string{{"a", "b"}, {"c", "d"}},
	)
	if err != nil {
		t.Fatalf("DebateMany failed: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].Question != "Q1?" || summaries[1].Question != "Q2?" {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}
}

func TestDebateManyMismatched(t *testing.T) {
	g := mustEngine(t)
	if _, err := g.DebateMany(context.Background(), []string{"q"}, nil); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestOpenEndedDebate(t *testing.T) {
	reply := func(string) string {
		return `{"answer":"Economic forces were decisive.","reasoning":"tax base collapsed"}`
	}
	cfg := testConfig(func(c *core.Config) {
		c.Protocol = core.ProtocolAdversarialPairs
		c.Agents = []adk.Agent{
			&fakeAgent{name: "econ", reply: reply},
			&fakeAgent{name: "pol", reply: reply},
		}
	})
	g, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	summary, err := g.Debate(ctx, "Why did Rome fall?", nil)
	if err != nil {
		t.Fatalf("open-ended Debate failed: %v", err)
	}
	if len(summary.OptionLabels) != 0 {
		t.Errorf("expected no option labels for open-ended debate, got %v", summary.OptionLabels)
	}
	if summary.PromptResult.After["econ"].Answer == "" {
		t.Error("expected a text answer for open-ended question")
	}
}

func TestPromptTemplateOverride(t *testing.T) {
	g := rawEngine(t, func(c *core.Config) {
		c.PromptTemplates.InitialPrompt = func(question string, options, labels []string, instr string) string {
			return "CUSTOM: " + question
		}
	})

	_, prompt, _ := g.buildInitialPrompt("Q?", []string{"a", "b"}, g.resolveCall())
	if !strings.HasPrefix(prompt, "CUSTOM: Q?") {
		t.Fatalf("custom prompt template not used: %q", prompt)
	}
}
