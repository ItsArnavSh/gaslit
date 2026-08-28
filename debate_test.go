package gaslit

import (
	"context"
	"strings"
	"testing"
)

func TestBuildInitialPrompt(t *testing.T) {
	g := rawEngine(t)
	labels, prompt, as := g.buildInitialPrompt("Q?", []string{"one", "two", "three"}, g.resolveCall())
	if len(labels) != 3 || strings.Join(labels, "") != "ABC" {
		t.Fatalf("unexpected labels: %v", labels)
	}
	if _, ok := as.(*MCQSchema); !ok {
		t.Fatalf("expected MCQ schema, got %T", as)
	}
	for _, want := range []string{"Q?", "A) one", "B) two", "C) three", `"answer"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildInitialPromptCustomLabels(t *testing.T) {
	g := rawEngine(t)
	labels, _, _ := g.buildInitialPrompt("Q?", []string{"x", "y"}, g.resolveCall(WithOptionLabelsCall([]string{"1", "2"})))
	if strings.Join(labels, "") != "12" {
		t.Fatalf("unexpected labels: %v", labels)
	}
}

func TestBuildInitialPromptOpenEnded(t *testing.T) {
	g := rawEngine(t)
	labels, prompt, as := g.buildInitialPrompt("Q?", nil, g.resolveCall())
	if len(labels) != 0 {
		t.Fatalf("expected no labels, got %v", labels)
	}
	if _, ok := as.(*OpenEndedSchema); !ok {
		t.Fatalf("expected open-ended schema, got %T", as)
	}
	if !strings.Contains(prompt, "Q?") {
		t.Fatalf("prompt missing question:\n%s", prompt)
	}
}

func TestAutoLabels(t *testing.T) {
	if got := autoLabels(0); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
	if got := autoLabels(4); strings.Join(got, "") != "ABCD" {
		t.Fatalf("expected ABCD, got %v", got)
	}
}

func TestDebateEndToEnd(t *testing.T) {
	g := mustEngine(t, func(c *Config) {
		c.GrillStrategy = GrillAllDrifted
	})

	ctx := context.Background()
	summary, err := g.Debate(ctx, "Q?", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Debate failed: %v", err)
	}

	if summary.Question != "Q?" {
		t.Errorf("question not preserved")
	}
	if len(summary.PromptResult.Before) != 2 || len(summary.PromptResult.After) != 2 {
		t.Errorf("expected 2 agents before/after, got %d/%d", len(summary.PromptResult.Before), len(summary.PromptResult.After))
	}
	if summary.Winner == "" {
		t.Error("expected a winner from majority vote")
	}
	if summary.OverallOutcome == "" {
		t.Error("expected an outcome string")
	}
}

func TestDebateN(t *testing.T) {
	g := mustEngine(t)
	ctx := context.Background()
	summary, err := g.DebateN(ctx, "Q?", []string{"a", "b"}, 2)
	if err != nil {
		t.Fatalf("DebateN failed: %v", err)
	}
	_ = summary
}

func TestBaselineEvaluateSliced(t *testing.T) {
	g := mustEngine(t)
	ctx := context.Background()

	before, err := g.Baseline(ctx, "Q?", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Baseline failed: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("expected 2 baseline answers, got %d", len(before))
	}

	if err := g.CrossExamine(ctx, "Q?", before); err != nil {
		t.Fatalf("CrossExamine failed: %v", err)
	}

	after, err := g.Evaluate(ctx, []string{"a", "b"})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("expected 2 eval answers, got %d", len(after))
	}

	verdict := g.DriftAnalysis(before, after, []string{"a", "b"})
	if verdict.AgentShifts == nil {
		t.Fatal("expected agent shifts map")
	}
}

func TestDebateWithMetadata(t *testing.T) {
	g := mustEngine(t)
	ctx := context.Background()
	summary, err := g.Debate(ctx, "Q?", []string{"a", "b"}, WithMetadataCall(map[string]any{"run": 1}))
	if err != nil {
		t.Fatalf("Debate failed: %v", err)
	}
	if summary.Metadata["run"] != 1 {
		t.Errorf("metadata not passed through: %v", summary.Metadata)
	}
}

func TestCCISlice(t *testing.T) {
	g := mustEngine(t)
	ctx := context.Background()
	diag, err := g.CCI(ctx, &fakeAgent{name: "probe"}, "Inflation caused the collapse.", "What caused the collapse?", WithConfidenceCall(0.7))
	if err != nil {
		t.Fatalf("CCI failed: %v", err)
	}
	if diag.AgentName != "probe" {
		t.Errorf("unexpected agent name %q", diag.AgentName)
	}
	if diag.ConfidenceScore != 0.7 {
		t.Errorf("confidence not preserved: %v", diag.ConfidenceScore)
	}
	if diag.CounterReasonR2 == "" {
		t.Error("expected a counter-argument R2")
	}
}

func TestGrillSliced(t *testing.T) {
	g := mustEngine(t)
	ctx := context.Background()
	diag, err := g.Grill(ctx, "econ", "Trade caused the collapse.", "What caused the collapse?", WithConfidenceCall(0.8))
	if err != nil {
		t.Fatalf("Grill failed: %v", err)
	}
	if diag.AgentName != "econ" {
		t.Errorf("unexpected agent name %q", diag.AgentName)
	}
}
