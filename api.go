package gaslit

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// New builds a Gaslit engine from a Config. Every unset field receives a
// default; see Config and the Option helpers. At least one agent is required,
// and agent names must be unique. Concurrent calls to Debate on the same engine
// are safe.
func New(ctx context.Context, cfg Config, opts ...Option) (*Gaslit, error) {
	for _, o := range opts {
		o(&cfg)
	}
	resolved, err := cfg.resolve()
	if err != nil {
		return nil, err
	}
	cfg = resolved

	g := &Gaslit{
		cfg:        cfg,
		logger:     cfg.Logger,
		states:     make(map[string]*agentState, len(cfg.Agents)),
		agentNames: make([]string, 0, len(cfg.Agents)),
		instr:      newInstrumentation(),
	}

	for _, a := range cfg.Agents {
		name := a.Name(ctx)
		if name == "" {
			return nil, fmt.Errorf("agent returned an empty name")
		}
		if _, dup := g.states[name]; dup {
			return nil, fmt.Errorf("duplicate agent name %q", name)
		}
		g.states[name] = &agentState{
			agent:   a,
			history: make([]*schema.Message, 0),
			cfg:     cfg.AgentOverrides[name],
		}
		g.agentNames = append(g.agentNames, name)
	}

	// Wire the LLM scorer (when used) to the configured NLI template.
	if scorer, ok := cfg.ConsistencyScorer.(*LLMScorer); ok && scorer.NLIProbe == nil {
		scorer.NLIProbe = cfg.PromptTemplates.NLIProbe
	}

	crossTool, err := g.createCrossExamineTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build cross-examine tool: %w", err)
	}
	tools := []tool.BaseTool{crossTool}
	tools = append(tools, cfg.ExtraTools...)

	supervisor, err := g.createSupervisorAgent(ctx, cfg.SupervisorModel, tools)
	if err != nil {
		return nil, fmt.Errorf("failed to build supervisor: %w", err)
	}
	g.supervisor = supervisor

	return g, nil
}
