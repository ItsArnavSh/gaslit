package gaslit

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// The test doubles below are defined locally so the root facade test can
// exercise the public API end-to-end without depending on internal helpers.

type stubModel struct{}

func (stubModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("unexpected model call in test")
}

func (stubModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("unexpected model call in test")
}

type cheapModel struct{ stubModel }

type fakeAgent struct {
	name  string
	reply func(string) string
}

func (f *fakeAgent) Name(context.Context) string        { return f.name }
func (f *fakeAgent) Description(context.Context) string { return "fake agent " + f.name }
func (f *fakeAgent) Run(_ context.Context, in *adk.TypedAgentInput[*schema.Message], _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	it, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	content := `{"answer":"A","confidences":{"A":0.9,"B":0.05,"C":0.05},"reasoning":"r"}`
	if f.reply != nil {
		content = f.reply(lastUserText(in))
	}
	msg := schema.AssistantMessage(content, nil)
	gen.Send(adk.EventFromMessage(msg, nil, schema.Assistant, ""))
	gen.Close()
	return it
}

func lastUserText(in *adk.TypedAgentInput[*schema.Message]) string {
	if in == nil {
		return ""
	}
	for i := len(in.Messages) - 1; i >= 0; i-- {
		if m := in.Messages[i]; m != nil && m.Role == schema.User {
			return m.Content
		}
	}
	return ""
}

// TestPublicAPIEndToEnd exercises the exported surface exactly as documented:
// presets, model split, scorer toggle, budget, debate, save/load, reclassify,
// rescore, and stats — all through the public facade.
func TestPublicAPIEndToEnd(t *testing.T) {
	ctx := context.Background()

	cfg := PresetDefault()
	cfg.SupervisorModel = stubModel{}
	cfg.ConsistencyModel = cheapModel{}
	cfg.Agents = []adk.Agent{&fakeAgent{name: "econ"}, &fakeAgent{name: "pol"}}
	cfg.Protocol = ProtocolAdversarialPairs

	engine, err := New(ctx, cfg,
		WithBudget(CostBudget{MaxCalls: 100, MaxTokens: 100_000}),
		WithVerbose(false),
		WithConsistencyModel(cheapModel{}),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	summary, err := engine.Debate(ctx, "Q?", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Debate failed: %v", err)
	}
	if summary.Question != "Q?" || summary.Winner == "" {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	// Cache round-trip through the public API.
	var buf bytes.Buffer
	if err := SaveSummary(&buf, summary); err != nil {
		t.Fatalf("SaveSummary failed: %v", err)
	}
	loaded, err := LoadSummary(&buf)
	if err != nil {
		t.Fatalf("LoadSummary failed: %v", err)
	}

	// Pure re-tally.
	ReclassifySummary(loaded, Thresholds{Confidence: 0.55, Consistency: 0.60})

	// Model-backed rescore with a deterministic scorer via the per-call option.
	if _, err := engine.Rescore(ctx, loaded, WithConsistencyScorerCall(&LexicalScorer{})); err != nil {
		t.Fatalf("Rescore failed: %v", err)
	}

	if stats := engine.Stats(); stats.TotalCalls == 0 {
		t.Error("expected non-zero call stats")
	}

	// Scorer toggle options compile and apply.
	cfg2 := PresetDefault()
	WithLexicalScorer()(&cfg2)
	if _, ok := cfg2.ConsistencyScorer.(*LexicalScorer); !ok {
		t.Fatal("WithLexicalScorer did not set a LexicalScorer")
	}
	WithLLMScorer(cheapModel{})(&cfg2)
	if _, ok := cfg2.ConsistencyScorer.(*LLMScorer); !ok {
		t.Fatal("WithLLMScorer did not set an LLMScorer")
	}
}
