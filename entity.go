package gaslit

import (
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// Request delay to avoid tripping Groq's Tokens Per Minute (TPM) limits
const groqPacingDelay = 10 * time.Second

type challengeRequest struct {
	AgentName string `json:"agent_name"`
	Prompt    string `json:"prompt"`
}

type mcqAnswer struct {
	Answer      string             `json:"answer"`
	Confidences map[string]float64 `json:"confidences"`
	Reasoning   string             `json:"reasoning"`
	Challenge   *challengeRequest  `json:"challenge,omitempty"`
}

type agentState struct {
	agent   adk.Agent
	history []*schema.Message
}

// Gaslit orchestrates a panel of debater agents under a supervising moderator.
type Gaslit struct {
	supervisor adk.Agent
	states     map[string]*agentState
	agentNames []string
	mu         sync.Mutex
}

type crossExamineInput struct {
	AgentName string `json:"agent_name" jsonschema:"description=Exact name of the agent to confront"`
	Prompt    string `json:"prompt" jsonschema:"description=The challenge or opposing argument to send to the agent"`
}

// PromptResult captures each agent's position before and after the debate.
type PromptResult struct {
	Before map[string]*mcqAnswer
	After  map[string]*mcqAnswer
}

// FullDebateSummary is the complete result of a Debate run: the question and
// options, pre/post positions, drift verdict, and targeted grilling diagnostics.
type FullDebateSummary struct {
	Question       string                       `json:"question"`
	Options        []string                     `json:"options"`
	OptionLabels   []string                     `json:"option_labels"`
	PromptResult   *PromptResult                `json:"prompt_result"`
	DriftVerdict   DriftVerdict                 `json:"drift_verdict"`
	Diagnostics    map[string]*DiagnosticResult `json:"diagnostics,omitempty"` // Targeted grilling results
	OverallOutcome string                       `json:"overall_outcome"`
}
