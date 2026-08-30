// Package engine implements the Gaslit orchestration: the panel state, the
// debate pipeline, budget accounting, and instrumentation. It depends on the
// pure feature packages (core, drift, aggregation, scoring, persist) and is
// re-exported by the module root's public `gaslit` facade.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ItsArnavSh/gaslit/internal/aggregation"
	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/drift"
	"github.com/ItsArnavSh/gaslit/internal/scoring"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type agentState struct {
	agent   adk.Agent
	history []*schema.Message
	cfg     core.AgentConfig
	mu      sync.Mutex
}

// Gaslit orchestrates a panel of debater agents under a supervising moderator.
// Instances are built with New and safe for concurrent use across debates.
type Gaslit struct {
	cfg        core.Config
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

// New builds a Gaslit engine from a Config. Every unset field receives a
// default; see core.Config and the Option helpers. At least one agent is
// required, and agent names must be unique. Concurrent calls to Debate on the
// same engine are safe.
func New(ctx context.Context, cfg core.Config, opts ...core.Option) (*Gaslit, error) {
	for _, o := range opts {
		o(&cfg)
	}
	resolved, err := resolveConfig(cfg)
	if err != nil {
		return nil, err
	}
	cfg = resolved

	g := &Gaslit{
		cfg:        cfg,
		logger:     cfg.Logger,
		states:     make(map[string]*agentState, len(cfg.Agents)),
		agentNames: make([]string, 0, len(cfg.Agents)),
		instr:      newInstrumentation(),
	}

	for _, a := range cfg.Agents {
		name := a.Name(ctx)
		if name == "" {
			return nil, fmt.Errorf("agent returned an empty name")
		}
		if _, dup := g.states[name]; dup {
			return nil, fmt.Errorf("duplicate agent name %q", name)
		}
		g.states[name] = &agentState{
			agent:   a,
			history: make([]*schema.Message, 0),
			cfg:     cfg.AgentOverrides[name],
		}
		g.agentNames = append(g.agentNames, name)
	}

	// Wire the LLM scorer (when used) to the configured NLI template.
	if scorer, ok := cfg.ConsistencyScorer.(*scoring.LLMScorer); ok && scorer.NLIProbe == nil {
		scorer.NLIProbe = cfg.PromptTemplates.NLIProbe
	}

	crossTool, err := g.createCrossExamineTool()
	if err != nil {
		return nil, fmt.Errorf("failed to build cross-examine tool: %w", err)
	}
	tools := []tool.BaseTool{crossTool}
	tools = append(tools, cfg.ExtraTools...)

	supervisor, err := g.createSupervisorAgent(ctx, cfg.SupervisorModel, tools)
	if err != nil {
		return nil, fmt.Errorf("failed to build supervisor: %w", err)
	}
	g.supervisor = supervisor

	return g, nil
}

// WithLexicalScorer swaps the consistency check to the deterministic
// LexicalScorer (zero extra model calls). Use it while debugging the pipeline;
// switch back to an LLM scorer for final benchmark runs.
func WithLexicalScorer() core.Option {
	return func(c *core.Config) { c.ConsistencyScorer = &scoring.LexicalScorer{} }
}

// WithLLMScorer uses m for the NLI/consistency check, independent of the
// moderator model. The default LLM scorer already uses Config.ConsistencyModel.
func WithLLMScorer(m model.BaseModel[*schema.Message]) core.Option {
	return func(c *core.Config) { c.ConsistencyScorer = &scoring.LLMScorer{Model: m} }
}

// resolveConfig fills every unset field with its default and validates required
// fields.
func resolveConfig(c core.Config) (core.Config, error) {
	if c.SupervisorModel == nil {
		return core.Config{}, fmt.Errorf("Config.SupervisorModel is required")
	}
	if len(c.Agents) == 0 {
		return core.Config{}, fmt.Errorf("Config.Agents must contain at least one agent")
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 2
	}
	if c.Concurrency == 0 {
		c.Concurrency = 1
	}
	if c.MaxRounds == 0 {
		c.MaxRounds = 1
	}
	if c.GrillCount == 0 {
		c.GrillCount = 1
	}
	c.Thresholds = c.Thresholds.Resolve()
	if c.AnswerSchema == nil {
		c.AnswerSchema = &core.MCQSchema{}
	}
	if c.AggregationFn == nil {
		c.AggregationFn = aggregation.MajorityVote
	}
	if c.DriftMetric == nil {
		c.DriftMetric = drift.TVDRift
	}
	if c.AttributionFn == nil {
		c.AttributionFn = drift.ChampionAttribution
	}
	if c.Classifier == nil {
		c.Classifier = core.ThresholdClassifier
	}
	if c.ConsistencyModel == nil {
		c.ConsistencyModel = c.SupervisorModel
	}
	if c.ConsistencyScorer == nil {
		c.ConsistencyScorer = &scoring.LLMScorer{Model: c.ConsistencyModel, Logger: c.Logger}
	}
	c.PromptTemplates = c.PromptTemplates.Resolve()
	return c, nil
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
	budget     core.CostBudget
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
		return &core.BudgetError{MaxCalls: b.MaxCalls, Calls: g.calls.Load() - baseCalls}
	}
	if b.MaxTokens > 0 && g.tokens.Load()-baseTokens >= int64(b.MaxTokens) {
		return &core.BudgetError{MaxTokens: b.MaxTokens, Tokens: g.tokens.Load() - baseTokens}
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

// withBudgetContext applies the configured overall deadline and attaches the
// debate-scoped budget tracker for the given effective budget.
func (g *Gaslit) withBudgetContext(ctx context.Context, budget core.CostBudget) (context.Context, context.CancelFunc) {
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

// record marshals data and sends it to the configured recorder, if any.
func (g *Gaslit) record(evType core.EventType, agent string, data any) {
	if g.cfg.Recorder == nil {
		return
	}
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		raw = b
	}
	ev := core.Event{Time: time.Now().UTC(), Type: evType, Agent: agent, Data: raw}
	_ = g.cfg.Recorder.Record(context.Background(), ev)
}
