package gaslit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func sanitizeHistoryMessage(msg *schema.Message) *schema.Message {
	if msg == nil {
		return nil
	}
	return &schema.Message{
		Role:    msg.Role,
		Content: msg.Content,
	}
}

func parseJSON(raw string) (*mcqAnswer, error) {
	end := strings.LastIndex(raw, "}")
	if end == -1 {
		return nil, fmt.Errorf("no closing brace found in response (raw: %s)", raw)
	}

	depth := 0
	start := -1
	for i := end; i >= 0; i-- {
		if raw[i] == '}' {
			depth++
		} else if raw[i] == '{' {
			depth--
			if depth == 0 {
				start = i
				break
			}
		}
	}

	if start == -1 {
		return nil, fmt.Errorf("no matching opening brace found for JSON object (raw: %s)", raw)
	}

	jsonStr := raw[start : end+1]

	var a mcqAnswer
	if err := json.Unmarshal([]byte(jsonStr), &a); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w (extracted: %s)", err, jsonStr)
	}

	if len(a.Confidences) == 0 {
		return nil, fmt.Errorf("confidences map is empty in parsed response: %s", jsonStr)
	}

	return &a, nil
}
func (g *Gaslit) sendToAgent(ctx context.Context, st *agentState, prompt string) (*schema.Message, error) {
	st.history = append(st.history, schema.UserMessage(prompt))

	iter := st.agent.Run(ctx, &adk.TypedAgentInput[*schema.Message]{
		Messages: st.history,
	})

	resp, err := consumeAgentOutput(iter)
	if err != nil {
		return nil, err
	}

	st.history = append(st.history, sanitizeHistoryMessage(resp))
	return resp, nil
}
