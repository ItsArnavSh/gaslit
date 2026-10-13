package groqmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var retryRegex = regexp.MustCompile(`(?:try again in|retry after)\s+([\d\.]+)s`)

type groqCleanTransport struct {
	base       http.RoundTripper
	maxRetries int
}

func (t *groqCleanTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var originalBody []byte
	var err error

	if req.Body != nil && req.Method == http.MethodPost {
		originalBody, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(originalBody, &payload); err == nil {
			if rawMsgs, ok := payload["messages"].([]any); ok {
				for _, rawMsg := range rawMsgs {
					if msgMap, ok := rawMsg.(map[string]any); ok {
						delete(msgMap, "reasoning_content")
					}
				}
			}
			delete(payload, "reasoning_effort")
			if tools, hasTools := payload["tools"].([]any); hasTools && len(tools) == 0 {
				delete(payload, "tools")
				delete(payload, "tool_choice")
			}
			if cleanedBytes, err := json.Marshal(payload); err == nil {
				originalBody = cleanedBytes
			}
		}
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	maxAttempts := t.maxRetries
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	for attempt := 0; attempt <= maxAttempts; attempt++ {
		if originalBody != nil {
			req.Body = io.NopCloser(bytes.NewReader(originalBody))
			req.ContentLength = int64(len(originalBody))
		}
		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		backoff := time.Duration(10+attempt*5) * time.Second
		if retryHeader := resp.Header.Get("Retry-After"); retryHeader != "" {
			if secs, parseErr := strconv.ParseFloat(retryHeader, 64); parseErr == nil {
				backoff = time.Duration(secs*float64(time.Second)) + (500 * time.Millisecond)
			}
		} else {
			matches := retryRegex.FindSubmatch(respBody)
			if len(matches) > 1 {
				if secs, parseErr := strconv.ParseFloat(string(matches[1]), 64); parseErr == nil {
					backoff = time.Duration(secs*float64(time.Second)) + (500 * time.Millisecond)
				}
			}
		}
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(backoff):
		}
	}
	return nil, fmt.Errorf("max retries exceeded")
}

// GetGroqModel returns a Groq chat model that emits *schema.Message.
func GetGroqModel(ctx context.Context, modelName string) (model.BaseModel[*schema.Message], error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		if data, err := os.ReadFile(os.Getenv("HOME") + "/.local/share/opencode/auth.json"); err == nil {
			var m map[string]map[string]any
			if json.Unmarshal(data, &m) == nil {
				if g, ok := m["groq"]; ok {
					if k, ok := g["key"].(string); ok && k != "" {
						apiKey = k
					}
				}
			}
		}
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY not set")
	}
	if modelName == "" {
		modelName = "groq/compound-mini"
	}
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  apiKey,
		Model:   modelName,
		HTTPClient: &http.Client{
			Transport: &groqCleanTransport{base: http.DefaultTransport, maxRetries: 4},
			Timeout:   120 * time.Second,
		},
	})
	if err != nil {
		return nil, err
	}
	return chatModel, nil
}
