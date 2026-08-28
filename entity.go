package gaslit

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type challengeRequest struct {
	AgentName string `json:"agent_name"`
	Prompt    string `json:"prompt"`
}

// Answer is a single agent's position on a question: its pick, per-option
// confidences (absent for open-ended questions), reasoning, and an optional
// nomination of another panelist for cross-examination.
type Answer struct {
	Answer      string             `json:"answer"`
	Confidences map[string]float64 `json:"confidences,omitempty"`
	Reasoning   string             `json:"reasoning"`
	Challenge   *challengeRequest  `json:"challenge,omitempty"`
}

type agentState struct {
	agent   adk.Agent
	history []*schema.Message
	cfg     AgentConfig
	mu      sync.Mutex
}

// Gaslit orchestrates a panel of debater agents under a supervising moderator.
// Instances are built with New and safe for concurrent use across debates.
type Gaslit struct {
	cfg        Config
	supervisor adk.Agent
	states     map[string]*agentState
	agentNames []string

	logger *slog.Logger
	instr  *instrumentation

	calls  atomic.Int64 // total model invocations (budget accounting)
	tokens atomic.Int64 // approximate tokens consumed (chars/4)
}

type crossExamineInput struct {
	AgentName string `json:"agent_name" jsonschema:"description=Exact name of the agent to confront"`
	Prompt    string `json:"prompt" jsonschema:"description=The challenge or opposing argument to send to the agent"`
}

// PromptResult captures each agent's position before and after the debate.
type PromptResult struct {
	Before map[string]*Answer
	After  map[string]*Answer
}

// FullDebateSummary is the complete result of a Debate run: the question and
// options, pre/post positions, drift verdict, targeted grilling diagnostics,
// and the aggregated winner.
type FullDebateSummary struct {
	Question       string                       `json:"question"`
	Options        []string                     `json:"options"`
	OptionLabels   []string                     `json:"option_labels"`
	PromptResult   *PromptResult                `json:"prompt_result"`
	DriftVerdict   DriftVerdict                 `json:"drift_verdict"`
	Diagnostics    map[string]*DiagnosticResult `json:"diagnostics,omitempty"` // Targeted grilling results
	Winner         string                       `json:"winner"`                // Aggregated winning option ("" if none)
	OverallOutcome string                       `json:"overall_outcome"`
	Metadata       map[string]any               `json:"metadata,omitempty"`
}

// redact applies the configured redaction function, if any.
func (g *Gaslit) redact(s string) string {
	if g.cfg.Redact == nil {
		return s
	}
	return g.cfg.Redact(s)
}

// pace sleeps for the configured pacing delay, honoring context cancellation.
func (g *Gaslit) pace(ctx context.Context) error {
	if g.cfg.PacingDelay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(g.cfg.PacingDelay):
		return nil
	}
}

// budgetTracker carries the effective budget and the usage baseline for a
// single debate, threaded through the context.
type budgetTracker struct {
	baseCalls  int64
	baseTokens int64
	budget     CostBudget
}

type budgetCtxKey struct{}

// checkBudget returns an error if the effective cost budget is exhausted. When
// called inside a debate (tracker present in ctx) it is measured relative to
// the debate start; otherwise it is measured against cumulative engine usage.
func (g *Gaslit) checkBudget(ctx context.Context) error {
	b := g.cfg.Budget
	baseCalls, baseTokens := int64(0), int64(0)

	if tr, ok := ctx.Value(budgetCtxKey{}).(*budgetTracker); ok && tr != nil {
		b = tr.budget
		baseCalls, baseTokens = tr.baseCalls, tr.baseTokens
	}

	if b.MaxCalls > 0 && g.calls.Load()-baseCalls >= int64(b.MaxCalls) {
		return &BudgetError{MaxCalls: b.MaxCalls, Calls: g.calls.Load() - baseCalls}
	}
	if b.MaxTokens > 0 && g.tokens.Load()-baseTokens >= int64(b.MaxTokens) {
		return &BudgetError{MaxTokens: b.MaxTokens, Tokens: g.tokens.Load() - baseTokens}
	}
	return nil
}

// accountCall registers one model invocation and its approximate token cost.
func (g *Gaslit) accountCall(agent string, content string) {
	g.calls.Add(1)
	tok := int64(len(content) / 4)
	g.tokens.Add(tok)
	if g.instr != nil {
		g.instr.recordAgentCall(agent)
	}
}

// BudgetError is returned when a CostBudget is exhausted.
type BudgetError struct {
	MaxCalls  int
	Calls     int64
	MaxTokens int
	Tokens    int64
}

func (e *BudgetError) Error() string {
	switch {
	case e.MaxCalls > 0:
		return "budget exceeded: maximum of " + itoa(e.MaxCalls) + " model calls reached"
	case e.MaxTokens > 0:
		return "budget exceeded: maximum of " + itoa(e.MaxTokens) + " tokens reached"
	default:
		return "budget exceeded"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// withBudgetContext applies the configured overall deadline and attaches the
// debate-scoped budget tracker for the given effective budget.
func (g *Gaslit) withBudgetContext(ctx context.Context, budget CostBudget) (context.Context, context.CancelFunc) {
	tr := &budgetTracker{
		baseCalls:  g.calls.Load(),
		baseTokens: g.tokens.Load(),
		budget:     budget,
	}
	ctx = context.WithValue(ctx, budgetCtxKey{}, tr)

	if d := budget.Deadline; d > 0 {
		return context.WithTimeout(ctx, d)
	}
	return context.WithCancel(ctx)
}
