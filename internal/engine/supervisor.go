package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func (g *Gaslit) createCrossExamineTool() (tool.BaseTool, error) {
	toolHandler := func(c context.Context, in *crossExamineInput) (string, error) {
		st, exists := g.states[in.AgentName]
		if !exists {
			return fmt.Sprintf("Agent %q not found. Available: %s", in.AgentName, strings.Join(g.agentNames, ", ")), nil
		}

		if g.cfg.Hooks.OnCrossExam != nil {
			g.cfg.Hooks.OnCrossExam(c, in.AgentName, in.Prompt)
		}
		g.record(core.EventCrossExam, in.AgentName, in)

		userMsg := fmt.Sprintf(g.cfg.PromptTemplates.CrossExamineLead, g.redact(in.Prompt), g.cfg.AnswerSchema.Instruction())
		respMsg, err := g.sendToAgent(c, st, userMsg)
		if err != nil {
			return "", fmt.Errorf("error from agent %s: %w", in.AgentName, err)
		}

		return fmt.Sprintf("[%s Statement]: %s", in.AgentName, respMsg.Content), nil
	}

	return utils.InferTool(
		"cross_examine_agent",
		fmt.Sprintf("Confront an expert with an opposing viewpoint. Available: [%s]", strings.Join(g.agentNames, ", ")),
		toolHandler,
	)
}

func (g *Gaslit) createSupervisorAgent(ctx context.Context, model model.BaseModel[*schema.Message], tools []tool.BaseTool) (adk.Agent, error) {
	names := strings.Join(g.agentNames, ", ")
	instruction := g.cfg.PromptTemplates.SupervisorSystem
	if strings.Contains(instruction, "%s") {
		instruction = fmt.Sprintf(instruction, names)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "moderator_supervisor",
		Description: "Interrogates agents in a structured debate.",
		Instruction: instruction,
		Model:       model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	})
}

// runSupervisor runs the moderator with a single user prompt, applying pacing,
// budget accounting, redaction, and an optional per-request timeout.
func (g *Gaslit) runSupervisor(ctx context.Context, prompt string) (*schema.Message, error) {
	if err := g.checkBudget(ctx); err != nil {
		return nil, err
	}
	if err := g.pace(ctx); err != nil {
		return nil, err
	}

	rctx, cancel := ctx, func() {}
	if g.cfg.RequestTimeout > 0 {
		rctx, cancel = context.WithTimeout(ctx, g.cfg.RequestTimeout)
	}
	defer cancel()

	iter := g.supervisor.Run(rctx, &adk.TypedAgentInput[*schema.Message]{
		Messages: []*schema.Message{schema.UserMessage(g.redact(prompt))},
	})
	resp, err := consumeAgentOutput(iter)
	if err != nil {
		return nil, err
	}
	g.accountCall("moderator_supervisor", resp.Content)
	return resp, nil
}
