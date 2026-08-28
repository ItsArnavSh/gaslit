package gaslit

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// stubModel implements model.BaseModel but fails on any actual call. It is only
// used to satisfy New's required SupervisorModel field in tests that never
// invoke the model directly.
type stubModel struct{}

func (stubModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("unexpected model call in test")
}

func (stubModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("unexpected model call in test")
}

// fakeAgent is a scriptable adk.Agent that returns a canned response.
type fakeAgent struct {
	name  string
	reply func(lastUserPrompt string) string
}

func (f *fakeAgent) Name(context.Context) string        { return f.name }
func (f *fakeAgent) Description(context.Context) string { return "fake agent " + f.name }
func (f *fakeAgent) Run(_ context.Context, in *adk.TypedAgentInput[*schema.Message], _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	it, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	content := `{"answer":"A","confidences":{"A":0.9,"B":0.05,"C":0.05},"reasoning":"default reasoning"}`
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

// testConfig returns a Config with stub dependencies and default settings.
func testConfig(cfgMod ...func(*Config)) Config {
	cfg := PresetDefault()
	cfg.SupervisorModel = stubModel{}
	cfg.Agents = []adk.Agent{
		&fakeAgent{name: "econ"},
		&fakeAgent{name: "pol"},
	}
	cfg.Protocol = ProtocolAdversarialPairs
	cfg.ConsistencyScorer = &LexicalScorer{}
	for _, m := range cfgMod {
		m(&cfg)
	}
	return cfg
}

// mustEngine builds a Gaslit via New from testConfig.
func mustEngine(t testingTB, cfgMod ...func(*Config)) *Gaslit {
	t.Helper()
	g, err := New(context.Background(), testConfig(cfgMod...))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return g
}

// rawEngine builds a Gaslit with a resolved Config directly, skipping the
// supervisor construction, for testing pure methods.
func rawEngine(t testingTB, cfgMod ...func(*Config)) *Gaslit {
	t.Helper()
	cfg, err := testConfig(cfgMod...).resolve()
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	g := &Gaslit{
		cfg:        cfg,
		logger:     cfg.Logger,
		states:     make(map[string]*agentState, len(cfg.Agents)),
		agentNames: make([]string, 0, len(cfg.Agents)),
		instr:      newInstrumentation(),
	}
	for _, a := range cfg.Agents {
		name := a.Name(context.Background())
		g.states[name] = &agentState{agent: a, history: make([]*schema.Message, 0), cfg: cfg.AgentOverrides[name]}
		g.agentNames = append(g.agentNames, name)
	}
	return g
}

type testingTB interface {
	Helper()
	Fatalf(format string, args ...any)
}
