package core

import (
	"log/slog"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// Config is the construction-time configuration for a Gaslit engine. The zero
// value is not usable directly; build one via Preset* helpers and/or the
// Option funcs. The engine applies defaults for every unset field.
type Config struct {
	// SupervisorModel is the model that powers the moderator (and, when
	// ConsistencyModel is unset, the default LLM consistency scorer). Required.
	SupervisorModel model.BaseModel[*schema.Message]
	// ConsistencyModel powers the default LLM consistency scorer. When nil it
	// falls back to SupervisorModel. Use a cheap model here and reserve
	// SupervisorModel for the moderator.
	ConsistencyModel model.BaseModel[*schema.Message]
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
	// contradict each other. Defaults to an LLM scorer over ConsistencyModel.
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
