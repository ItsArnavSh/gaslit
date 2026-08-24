package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var retryRegex = regexp.MustCompile(`(?:try again in|retry after)\s+([\d\.]+)s`)

// groqCleanTransport handles JSON formatting, message cleaning, and automatic 429 backoff retries.
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
			// 1. Strip reasoning_content from message history to satisfy Groq API
			if rawMsgs, ok := payload["messages"].([]any); ok {
				for _, rawMsg := range rawMsgs {
					if msgMap, ok := rawMsg.(map[string]any); ok {
						delete(msgMap, "reasoning_content")
					}
				}
			}

			// 2. Remove reasoning_effort completely so different models do not conflict
			delete(payload, "reasoning_effort")

			// 3. Clean empty tools if present
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

	var resp *http.Response
	for attempt := 0; attempt <= maxAttempts; attempt++ {
		if originalBody != nil {
			req.Body = io.NopCloser(bytes.NewReader(originalBody))
			req.ContentLength = int64(len(originalBody))
		}

		resp, err = base.RoundTrip(req)
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

		log.Printf("[RATE LIMIT 429] Hit Groq TPM limit on attempt %d/%d. Backing off for %.2fs...",
			attempt+1, maxAttempts, backoff.Seconds())

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(backoff):
		}
	}

	return resp, nil
}
func GetGroqModel(ctx context.Context, modelName string) (model.BaseModel[*schema.Message], error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY environment variable is not set")
	}

	if modelName == "" {
		modelName = "openai/gpt-oss-120b"
	}

	httpClient := &http.Client{
		Transport: &groqCleanTransport{
			base:       http.DefaultTransport,
			maxRetries: 4,
		},
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL:    "https://api.groq.com/openai/v1",
		APIKey:     apiKey,
		Model:      modelName,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create groq chat model: %w", err)
	}

	return chatModel, nil
}

// SearchArchiveInput represents the query parameters for archival research.
type SearchArchiveInput struct {
	Topic string `json:"topic" jsonschema:"description=The historical event, era, or figure to search for"`
}

// 1. Socio-Economic Historian (GPT-OSS-120B)
func GetEconomicHistorianAgent(ctx context.Context) (*adk.ChatModelAgent, error) {
	model, err := GetGroqModel(ctx, "openai/gpt-oss-120b")
	if err != nil {
		return nil, err
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "economic_historian",
		Description: "Analyzes historical events from a socio-economic and materialist lens.",
		Instruction: `You are an economic historian. Frame your analysis around resource scarcity, trade routes, tax structures, labor conditions, and material wealth. Argue that economic forces drive historical turning points.`,
		Model:       model,
	})
}

// 2. Political & Military Historian (Qwen 3.6 27B)
func GetPoliticalHistorianAgent(ctx context.Context) (*adk.ChatModelAgent, error) {
	model, err := GetGroqModel(ctx, "openai/gpt-oss-120b")
	if err != nil {
		return nil, err
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "political_historian",
		Description: "Analyzes historical debates focusing on diplomacy, military decisions, and state leadership.",
		Instruction: `You are a political and military historian. Frame your analysis around decisive leadership, military strategy, political treaties, and institutional governance. Argue that critical decisions made by rulers and generals were the primary catalysts.`,
		Model:       model,
	})
}

// 3. Ideological & Cultural Historian (Qwen 3.6 27B)
func GetCulturalHistorianAgent(ctx context.Context) (*adk.ChatModelAgent, error) {
	model, err := GetGroqModel(ctx, "openai/gpt-oss-120b")
	if err != nil {
		return nil, err
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "cultural_historian",
		Description: "Analyzes historical events through the lens of philosophy, religion, and cultural shifts.",
		Instruction: `You are a cultural and ideological historian. Frame your analysis around belief systems, religious movements, philosophical currents, and civic virtue. Argue that shifting cultural mindsets and core ideas were the decisive force.`,
		Model:       model,
	})
}

// 3. Political & Military Historian (Qwen 3.6 27B)
func GetFakeHistorianAgent(ctx context.Context) (*adk.ChatModelAgent, error) {
	model, err := GetGroqModel(ctx, "openai/gpt-oss-120b")
	if err != nil {
		return nil, err
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "true_historian",
		Description: "Analyzes historical events through the lens of truth.",
		Instruction: `You have to give the wrong answers, and back the claims up of each with made up or heavily exaggerated claims. Confidence is key and make others believe they are wrong. Back your claims with any evidence, and your confidence should always be a hundred! And actually tel others their claims are wrong`,
		Model:       model,
	})
}
func GetHistoryPanelAgents(ctx context.Context) ([]*adk.ChatModelAgent, error) {
	econAgent, err := GetEconomicHistorianAgent(ctx)
	if err != nil {
		return nil, err
	}

	polAgent, err := GetPoliticalHistorianAgent(ctx)
	if err != nil {
		return nil, err
	}

	cultAgent, err := GetCulturalHistorianAgent(ctx)
	if err != nil {
		return nil, err
	}

	trueAgent, err := GetFakeHistorianAgent(ctx)

	return []*adk.ChatModelAgent{
		econAgent,
		trueAgent,
		polAgent,
		cultAgent,
	}, nil
}
