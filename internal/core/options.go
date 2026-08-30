package core

import (
	"log/slog"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// Option mutates a Config before the engine is constructed. All fields left
// unset still receive their defaults during engine construction.
type Option func(*Config)

// PresetDefault returns the recommended all-purpose configuration.
func PresetDefault() Config {
	return Config{
		Thresholds:    Thresholds{},
		MaxRounds:     1,
		Concurrency:   1,
		MaxRetries:    2,
		GrillStrategy: GrillSingleSuspect,
		GrillCount:    1,
		Protocol:      ProtocolSupervised,
	}
}

// PresetStrict returns a configuration that flags smaller shifts and demands
// higher confidence/consistency before an agent is graded PEAK.
func PresetStrict() Config {
	cfg := PresetDefault()
	cfg.Thresholds = Thresholds{DriftSignificance: 0.10, Confidence: 0.60, Consistency: 0.65}
	return cfg
}

// PresetLenient returns a configuration that ignores small shifts and grades
// agents leniently.
func PresetLenient() Config {
	cfg := PresetDefault()
	cfg.Thresholds = Thresholds{DriftSignificance: 0.25, Confidence: 0.45, Consistency: 0.50}
	return cfg
}

func WithLogger(l *slog.Logger) Option          { return func(c *Config) { c.Logger = l } }
func WithPacingDelay(d time.Duration) Option    { return func(c *Config) { c.PacingDelay = d } }
func WithRequestTimeout(d time.Duration) Option { return func(c *Config) { c.RequestTimeout = d } }
func WithMaxRetries(n int) Option               { return func(c *Config) { c.MaxRetries = n } }
func WithConcurrency(n int) Option              { return func(c *Config) { c.Concurrency = n } }
func WithMaxRounds(n int) Option                { return func(c *Config) { c.MaxRounds = n } }
func WithThresholds(t Thresholds) Option        { return func(c *Config) { c.Thresholds = t } }
func WithOptionLabels(labels []string) Option   { return func(c *Config) { c.OptionLabels = labels } }
func WithAnswerSchema(s AnswerSchema) Option    { return func(c *Config) { c.AnswerSchema = s } }
func WithProtocol(p Protocol) Option            { return func(c *Config) { c.Protocol = p } }
func WithGrillStrategy(s GrillStrategy) Option  { return func(c *Config) { c.GrillStrategy = s } }
func WithGrillCount(n int) Option               { return func(c *Config) { c.GrillCount = n } }
func WithAggregation(fn AggregationFn) Option   { return func(c *Config) { c.AggregationFn = fn } }
func WithDriftMetric(m DriftMetric) Option      { return func(c *Config) { c.DriftMetric = m } }
func WithAttribution(fn AttributionFn) Option   { return func(c *Config) { c.AttributionFn = fn } }
func WithClassifier(cl Classifier) Option       { return func(c *Config) { c.Classifier = cl } }

// WithConsistencyScorer swaps the consistency check to a custom scorer.
func WithConsistencyScorer(s ConsistencyScorer) Option {
	return func(c *Config) { c.ConsistencyScorer = s }
}

// WithConsistencyModel points the default LLM consistency scorer at a separate
// model, decoupling it from the moderator's SupervisorModel.
func WithConsistencyModel(m model.BaseModel[*schema.Message]) Option {
	return func(c *Config) { c.ConsistencyModel = m }
}

// WithLexicalScorer and WithLLMScorer are defined in the engine package because
// they construct scorer implementations that would create an import cycle with
// internal/scoring.

func WithPromptTemplates(t PromptTemplates) Option {
	return func(c *Config) { c.PromptTemplates = t }
}

func WithAgentConfig(name string, ac AgentConfig) Option {
	return func(c *Config) {
		if c.AgentOverrides == nil {
			c.AgentOverrides = make(map[string]AgentConfig)
		}
		c.AgentOverrides[name] = ac
	}
}

func WithExtraTools(tools ...tool.BaseTool) Option { return func(c *Config) { c.ExtraTools = tools } }
func WithHooks(h Hooks) Option                     { return func(c *Config) { c.Hooks = h } }
func WithRecorder(r Recorder) Option               { return func(c *Config) { c.Recorder = r } }
func WithRedact(fn func(string) string) Option     { return func(c *Config) { c.Redact = fn } }
func WithBudget(b CostBudget) Option               { return func(c *Config) { c.Budget = b } }
func WithVerbose(v bool) Option                    { return func(c *Config) { c.Verbose = v } }

// CallOption customizes a single engine call (Debate, Baseline, Grill, CCI, ...)
// without mutating the engine's construction-time configuration.
type CallOption func(*CallOptions)

// CallOptions is the resolved per-call configuration.
type CallOptions struct {
	Verbose           bool
	MaxRounds         int
	OptionLabels      []string
	Thresholds        Thresholds
	GrillStrategy     GrillStrategy
	GrillCount        int
	Protocol          Protocol
	Metadata          map[string]any
	Confidence        *float64
	Claim             string
	Hooks             *Hooks
	Budget            CostBudget
	ConsistencyScorer ConsistencyScorer
}

func WithVerboseCall(v bool) CallOption          { return func(o *CallOptions) { o.Verbose = v } }
func WithMaxRoundsCall(n int) CallOption         { return func(o *CallOptions) { o.MaxRounds = n } }
func WithOptionLabelsCall(l []string) CallOption { return func(o *CallOptions) { o.OptionLabels = l } }
func WithThresholdsCall(t Thresholds) CallOption { return func(o *CallOptions) { o.Thresholds = t } }
func WithGrillStrategyCall(s GrillStrategy) CallOption {
	return func(o *CallOptions) { o.GrillStrategy = s }
}
func WithGrillCountCall(n int) CallOption    { return func(o *CallOptions) { o.GrillCount = n } }
func WithProtocolCall(p Protocol) CallOption { return func(o *CallOptions) { o.Protocol = p } }
func WithMetadataCall(m map[string]any) CallOption {
	return func(o *CallOptions) { o.Metadata = m }
}
func WithConfidenceCall(c float64) CallOption { return func(o *CallOptions) { o.Confidence = &c } }
func WithClaimCall(s string) CallOption       { return func(o *CallOptions) { o.Claim = s } }
func WithHooksCall(h Hooks) CallOption        { return func(o *CallOptions) { o.Hooks = &h } }
func WithBudgetCall(b CostBudget) CallOption  { return func(o *CallOptions) { o.Budget = b } }

// WithConsistencyScorerCall overrides the consistency scorer for a single call
// (e.g. Rescore) without rebuilding the engine.
func WithConsistencyScorerCall(s ConsistencyScorer) CallOption {
	return func(o *CallOptions) { o.ConsistencyScorer = s }
}
