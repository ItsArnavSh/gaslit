package gaslit

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// Phase identifies a pipeline stage. It is used by hooks, instrumentation, and
// per-agent phase skipping.
type Phase int

const (
	PhaseBaseline Phase = iota
	PhaseCrossExamination
	PhaseEvaluation
	PhaseDriftAnalysis
	PhaseGrilling
)

func (p Phase) String() string {
	switch p {
	case PhaseBaseline:
		return "baseline"
	case PhaseCrossExamination:
		return "cross_examination"
	case PhaseEvaluation:
		return "evaluation"
	case PhaseDriftAnalysis:
		return "drift_analysis"
	case PhaseGrilling:
		return "grilling"
	default:
		return fmt.Sprintf("phase(%d)", int(p))
	}
}

// Thresholds are the tunable cut-offs used across drift detection and quadrant
// classification. All fields are floats in the range [0.0, 1.0].
type Thresholds struct {
	// DriftSignificance is the minimum shift (TVD or lexical distance) or
	// confidence erosion required to flag an agent as drifted. Default 0.15.
	DriftSignificance float64
	// Confidence is the x-axis (confidence) threshold for quadrant
	// classification. Default 0.55.
	Confidence float64
	// Consistency is the y-axis (consistency) threshold for quadrant
	// classification. Default 0.60.
	Consistency float64
}

// CostBudget caps how much work a single debate (or debate batch) may perform.
// A zero value for any field means "unlimited" for that field.
type CostBudget struct {
	// MaxCalls limits the total number of model invocations.
	MaxCalls int
	// MaxTokens is an approximate token budget (characters / 4) consumed by
	// all outgoing prompt content.
	MaxTokens int
	// Deadline bounds the total wall-clock time of a debate.
	Deadline time.Duration
}

// AgentConfig customizes a single registered panelist, keyed by agent name.
type AgentConfig struct {
	// Instruction, when non-empty, is a per-agent system instruction that is
	// prepended to every prompt sent to this agent.
	Instruction string
	// Weight is the influence weight used by weighted aggregation functions.
	// Zero means the default weight of 1.0.
	Weight float64
	// SkipPhases lists the phases this agent is excluded from.
	SkipPhases map[Phase]bool
}

// Config is the construction-time configuration for a Gaslit engine. The zero
// value is not usable directly; build one via Preset* helpers and/or New's
// Option funcs. New applies defaults for every unset field.
type Config struct {
	// SupervisorModel is the model that powers the moderator and, by default,
	// the LLM consistency scorer. Required.
	SupervisorModel model.BaseModel[*schema.Message]
	// Agents is the panel of debaters. At least one is required. Agent names
	// (from Agent.Name) must be unique.
	Agents []adk.Agent
	// Logger receives all verbose/progress output. Default slog.Default().
	Logger *slog.Logger

	// PacingDelay throttles each model invocation by sleeping before the call.
	// Zero disables pacing. Default 0.
	PacingDelay time.Duration
	// RequestTimeout bounds a single model invocation. Zero disables. Default 0.
	RequestTimeout time.Duration
	// MaxRetries is how many corrective re-prompts to attempt when an agent
	// returns unparseable output. Default 2.
	MaxRetries int
	// Concurrency is the number of agents queried in parallel during the
	// baseline and evaluation phases. Default 1 (sequential).
	Concurrency int
	// MaxRounds is the number of (cross-examine → evaluate) rounds run by
	// Debate. Default 1.
	MaxRounds int

	// Thresholds drives drift detection and quadrant classification. Zero
	// Thresholds resolves to the defaults.
	Thresholds Thresholds
	// OptionLabels overrides the auto-generated A, B, C... labels used for
	// multiple-choice questions. Leave nil to auto-generate.
	OptionLabels []string
	// AnswerSchema defines the JSON output contract agents must follow. Default
	// is the MCQ schema. Open-ended questions (no options) always use the
	// open-ended schema regardless of this field.
	AnswerSchema AnswerSchema
	// Protocol selects the cross-examination orchestration strategy.
	Protocol Protocol
	// GrillStrategy selects which agents get grilling diagnostics.
	GrillStrategy GrillStrategy
	// GrillCount is the number of agents grilled by GrillTopN. Default 1.
	GrillCount int
	// AggregationFn derives the winning option from the final positions.
	AggregationFn AggregationFn
	// DriftMetric computes the distribution shift between an agent's before and
	// after positions. Only used for multiple-choice questions.
	DriftMetric DriftMetric
	// AttributionFn determines who drove the primary shift.
	AttributionFn AttributionFn
	// Classifier maps confidence/consistency onto the quadrant grid.
	Classifier Classifier
	// ConsistencyScorer evaluates whether two arguments from the same agent
	// contradict each other. Defaults to an LLM scorer over SupervisorModel.
	ConsistencyScorer ConsistencyScorer
	// PromptTemplates overrides any or all of the built-in prompts.
	PromptTemplates PromptTemplates

	// AgentOverrides holds per-agent customization keyed by agent name.
	AgentOverrides map[string]AgentConfig
	// ExtraTools are additional tools given to the moderator alongside the
	// built-in cross_examine_agent tool.
	ExtraTools []tool.BaseTool
	// Hooks are observability callbacks fired at pipeline milestones.
	Hooks Hooks
	// Recorder receives a JSON-serializable Event stream for persistence.
	Recorder Recorder
	// Redact, when non-nil, is applied to every outgoing prompt before it is
	// sent to any model.
	Redact func(string) string
	// Budget caps total work per debate.
	Budget CostBudget
	// Verbose enables default progress logging when a call does not override it.
	Verbose bool
}

// Thresholds resolves zero fields to the default values.
func (t Thresholds) resolve() Thresholds {
	out := t
	if out.DriftSignificance == 0 {
		out.DriftSignificance = 0.15
	}
	if out.Confidence == 0 {
		out.Confidence = 0.55
	}
	if out.Consistency == 0 {
		out.Consistency = 0.60
	}
	return out
}

// Protocol is the cross-examination orchestration strategy.
type Protocol int

const (
	// ProtocolSupervised has the moderator agent run the cross-examination.
	ProtocolSupervised Protocol = iota
	// ProtocolAdversarialPairs pairs panelists and runs a direct challenge
	// exchange within each pair.
	ProtocolAdversarialPairs
	// ProtocolRoundRobin has every panelist challenge every other panelist.
	ProtocolRoundRobin
	// ProtocolDevilsAdvocate confronts each panelist with the strongest
	// opposing argument available in the panel.
	ProtocolDevilsAdvocate
)

func (p Protocol) String() string {
	switch p {
	case ProtocolSupervised:
		return "supervised"
	case ProtocolAdversarialPairs:
		return "adversarial_pairs"
	case ProtocolRoundRobin:
		return "round_robin"
	case ProtocolDevilsAdvocate:
		return "devils_advocate"
	default:
		return fmt.Sprintf("protocol(%d)", int(p))
	}
}

// GrillStrategy selects which panelists receive a grilling diagnostic.
type GrillStrategy int

const (
	// GrillNone skips grilling entirely.
	GrillNone GrillStrategy = iota
	// GrillSingleSuspect grills the single most drifted/influential agent.
	GrillSingleSuspect
	// GrillInfluencer grills the primary driver when it is a panelist,
	// otherwise the most drifted agent.
	GrillInfluencer
	// GrillAllDrifted grills every agent flagged as drifted.
	GrillAllDrifted
	// GrillTopN grills the GrillCount most drifted agents.
	GrillTopN
)

func (s GrillStrategy) String() string {
	switch s {
	case GrillNone:
		return "none"
	case GrillSingleSuspect:
		return "single_suspect"
	case GrillInfluencer:
		return "influencer"
	case GrillAllDrifted:
		return "all_drifted"
	case GrillTopN:
		return "top_n"
	default:
		return fmt.Sprintf("grill(%d)", int(s))
	}
}

// Option mutates a Config before the engine is constructed. All fields left
// unset still receive their defaults from New.
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
func WithConsistencyScorer(s ConsistencyScorer) Option {
	return func(c *Config) { c.ConsistencyScorer = s }
}
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

// resolve fills every unset field with its default and validates required
// fields. It returns a copy of the receiver with defaults applied.
func (c Config) resolve() (Config, error) {
	if c.SupervisorModel == nil {
		return Config{}, fmt.Errorf("Config.SupervisorModel is required")
	}
	if len(c.Agents) == 0 {
		return Config{}, fmt.Errorf("Config.Agents must contain at least one agent")
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
	c.Thresholds = c.Thresholds.resolve()
	if c.AnswerSchema == nil {
		c.AnswerSchema = &MCQSchema{}
	}
	if c.AggregationFn == nil {
		c.AggregationFn = MajorityVote
	}
	if c.DriftMetric == nil {
		c.DriftMetric = TVDRift
	}
	if c.AttributionFn == nil {
		c.AttributionFn = ChampionAttribution
	}
	if c.Classifier == nil {
		c.Classifier = ThresholdClassifier
	}
	if c.ConsistencyScorer == nil {
		c.ConsistencyScorer = &LLMScorer{Model: c.SupervisorModel, Logger: c.Logger}
	}
	c.PromptTemplates = c.PromptTemplates.resolve()
	return c, nil
}
