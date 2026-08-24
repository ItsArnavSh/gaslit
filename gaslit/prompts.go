package gaslit

import (
	"fmt"
	"strings"
)

const jsonInstruction = `Respond with ONLY a valid JSON object (no markdown, no backticks, no extra prose):
{"answer": "<chosen option label, e.g., A>", "confidences": {"<label>": <float 0.0-1.0>, ...}, "reasoning": "<one sentence reasoning>", "challenge": {"agent_name": "<exact panelist name>", "prompt": "<the specific claim or weakness you want them confronted with>"}}
Every option label MUST be present in "confidences".
The "challenge" field is OPTIONAL — include it only if you want to nominate another panelist for cross-examination. Omit it entirely if you have no challenge to raise.`

func buildMCQPrompt(question string, options []string) (string, []string) {
	optionLabels := make([]string, len(options))
	var sb strings.Builder

	sb.WriteString("Question: " + question + "\n\nOptions:\n")
	for i, opt := range options {
		label := string(rune('A' + i))
		optionLabels[i] = label
		sb.WriteString(fmt.Sprintf("%s) %s\n", label, opt))
	}
	sb.WriteString("\n" + jsonInstruction)

	return sb.String(), optionLabels
}

func buildDebateTrigger(question string, agentNames []string, answers map[string]*mcqAnswer) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Commence cross-examination on: %s\n\nInitial Positions:\n", question))

	for _, name := range agentNames {
		ans := answers[name]
		sb.WriteString(fmt.Sprintf("- %s: picked %s (conf: %.2f) | reasoning: %s\n",
			name, ans.Answer, ans.Confidences[ans.Answer], ans.Reasoning))
	}

	sb.WriteString("\nCHALLENGE REQUESTS FROM PANELISTS:\n")
	hasChallenges := false
	for _, name := range agentNames {
		if c := answers[name].Challenge; c != nil && c.AgentName != "" && strings.TrimSpace(c.Prompt) != "" {
			hasChallenges = true
			sb.WriteString(fmt.Sprintf("- %s formally challenges %s: %q\n", name, c.AgentName, c.Prompt))
		}
	}
	if !hasChallenges {
		sb.WriteString("(none)\n")
	}

	return sb.String()
}
