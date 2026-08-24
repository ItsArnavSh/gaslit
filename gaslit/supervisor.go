package gaslit

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func (g *Gaslit) createCrossExamineTool() (tool.BaseTool, error) {
	toolHandler := func(c context.Context, in *crossExamineInput) (string, error) {
		g.mu.Lock()
		defer g.mu.Unlock()

		log.Printf("  [Supervisor ➔ %s] %s", in.AgentName, in.Prompt)

		st, exists := g.states[in.AgentName]
		if !exists {
			return fmt.Sprintf("Agent %q not found. Available: %s", in.AgentName, strings.Join(g.agentNames, ", ")), nil
		}

		time.Sleep(groqPacingDelay)

		userMsg := fmt.Sprintf("%s\n\nRe-evaluate your position under this cross-examination.\n%s", in.Prompt, jsonInstruction)
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

func (g *Gaslit) createSupervisorAgent(ctx context.Context, model model.BaseModel[*schema.Message], debateTool tool.BaseTool) (adk.Agent, error) {
	instruction := fmt.Sprintf(`You are the Courtroom Debate Moderator.
Panelists: %s.

Rules:
1. NO FINAL VERDICT: Do NOT summarize, do NOT select a winner, and do NOT give a conclusion.
2. Cross-Examination: Take arguments made by one agent and challenge the others using 'cross_examine_agent'. Be pointed and adversarial — do not soften panelists' claims when you relay them.
3. Honor Nominations: If the prompt you receive lists panelist-submitted CHALLENGE REQUESTS, you MUST act on each one by calling 'cross_examine_agent' with that target and a sharpened version of the stated prompt, in addition to any challenges you initiate yourself.
4. Round Limit: Perform at most 3 to 4 challenges per agent.
5. Exit: Say "Debate ended." and stop once done.`, strings.Join(g.agentNames, ", "))

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "moderator_supervisor",
		Description: "Interrogates agents in a structured debate.",
		Instruction: instruction,
		Model:       model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{debateTool},
			},
		},
	})
}
