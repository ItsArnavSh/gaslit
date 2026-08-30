package engine

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func consumeAgentOutput(iter *adk.AsyncIterator[*adk.AgentEvent]) (*schema.Message, error) {
	if iter == nil {
		return nil, fmt.Errorf("nil iterator received")
	}

	var latestAssistantMsg *schema.Message
	var fallbackChunks strings.Builder

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return nil, event.Err
		}

		msg, _, err := adk.GetMessage(event)
		if err == nil && msg != nil {
			if strings.TrimSpace(msg.Content) != "" {
				latestAssistantMsg = msg
				fallbackChunks.WriteString(msg.Content)
			}
		}
	}

	if latestAssistantMsg != nil && len(strings.TrimSpace(latestAssistantMsg.Content)) > 0 {
		return &schema.Message{
			Role:    schema.Assistant,
			Content: latestAssistantMsg.Content,
		}, nil
	}

	accumulated := strings.TrimSpace(fallbackChunks.String())
	if len(accumulated) > 0 {
		return &schema.Message{
			Role:    schema.Assistant,
			Content: accumulated,
		}, nil
	}

	return nil, fmt.Errorf("agent completed without returning any text content")
}
