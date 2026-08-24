package gaslit

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

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
	log.Printf("[GASLIT] Framework initialized with %d debater agents.", len(agents))
	return g, nil
}
