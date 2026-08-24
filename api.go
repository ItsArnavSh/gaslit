package gaslit

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Init builds a Gaslit engine around the supplied supervisor model and debater
// agents. At least one agent is required. The returned engine serializes the
// baseline/evaluation phases, supervised cross-examination, and drift analysis.
//
// The agents are keyed by their Name, which must be unique. Concurrent calls to
// Debate on the same engine are safe.
func Init(ctx context.Context, supervisorModel model.BaseModel[*schema.Message], agents []adk.Agent) (*Gaslit, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("at least one agent must be provided")
	}

	g := &Gaslit{
		states:     make(map[string]*agentState, len(agents)),
		agentNames: make([]string, 0, len(agents)),
	}

	for _, a := range agents {
		name := a.Name(ctx)
		g.states[name] = &agentState{
			agent:   a,
			history: make([]*schema.Message, 0),
		}
		g.agentNames = append(g.agentNames, name)
	}

	debateTool, err := g.createCrossExamineTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build debate tool: %w", err)
	}

	supervisorAgent, err := g.createSupervisorAgent(ctx, supervisorModel, debateTool)
	if err != nil {
		return nil, fmt.Errorf("failed to build supervisor: %w", err)
	}

	g.supervisor = supervisorAgent
	return g, nil
}
