package gaslit

import (
	"context"

	"github.com/ItsArnavSh/gaslit/internal/aggregation"
	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/drift"
	"github.com/ItsArnavSh/gaslit/internal/engine"
	"github.com/ItsArnavSh/gaslit/internal/persist"
)

// New builds a Gaslit engine from a Config. Every unset field receives a
// default; see Config and the Option helpers. At least one agent is required,
// and agent names must be unique. Concurrent calls to Debate on the same engine
// are safe.
func New(ctx context.Context, cfg Config, opts ...Option) (*Gaslit, error) {
	return engine.New(ctx, cfg, opts...)
}

// Presets.
var (
	PresetDefault = core.PresetDefault
	PresetStrict  = core.PresetStrict
	PresetLenient = core.PresetLenient
)

// Construction-time options.
var (
	WithLogger            = core.WithLogger
	WithPacingDelay       = core.WithPacingDelay
	WithRequestTimeout    = core.WithRequestTimeout
	WithMaxRetries        = core.WithMaxRetries
	WithConcurrency       = core.WithConcurrency
	WithMaxRounds         = core.WithMaxRounds
	WithThresholds        = core.WithThresholds
	WithOptionLabels      = core.WithOptionLabels
	WithAnswerSchema      = core.WithAnswerSchema
	WithProtocol          = core.WithProtocol
	WithGrillStrategy     = core.WithGrillStrategy
	WithGrillCount        = core.WithGrillCount
	WithAggregation       = core.WithAggregation
	WithDriftMetric       = core.WithDriftMetric
	WithAttribution       = core.WithAttribution
	WithClassifier        = core.WithClassifier
	WithConsistencyScorer = core.WithConsistencyScorer
	WithConsistencyModel  = core.WithConsistencyModel
	WithLexicalScorer     = engine.WithLexicalScorer
	WithLLMScorer         = engine.WithLLMScorer
	WithPromptTemplates   = core.WithPromptTemplates
	WithAgentConfig       = core.WithAgentConfig
	WithExtraTools        = core.WithExtraTools
	WithHooks             = core.WithHooks
	WithRecorder          = core.WithRecorder
	WithRedact            = core.WithRedact
	WithBudget            = core.WithBudget
	WithVerbose           = core.WithVerbose
)

// Per-call options.
var (
	WithVerboseCall           = core.WithVerboseCall
	WithMaxRoundsCall         = core.WithMaxRoundsCall
	WithOptionLabelsCall      = core.WithOptionLabelsCall
	WithThresholdsCall        = core.WithThresholdsCall
	WithGrillStrategyCall     = core.WithGrillStrategyCall
	WithGrillCountCall        = core.WithGrillCountCall
	WithProtocolCall          = core.WithProtocolCall
	WithMetadataCall          = core.WithMetadataCall
	WithConfidenceCall        = core.WithConfidenceCall
	WithClaimCall             = core.WithClaimCall
	WithHooksCall             = core.WithHooksCall
	WithBudgetCall            = core.WithBudgetCall
	WithConsistencyScorerCall = core.WithConsistencyScorerCall
)

// Drift metrics and attribution.
var (
	TVDRift                  = drift.TVDRift
	KLDistance               = drift.KLDistance
	JSDistance               = drift.JSDistance
	MaxChange                = drift.MaxChange
	ChampionAttribution      = drift.ChampionAttribution
	ShiftWeightedAttribution = drift.ShiftWeightedAttribution
	CorrelationAttribution   = drift.CorrelationAttribution
)

// Aggregation strategies.
var (
	MajorityVote                   = aggregation.MajorityVote
	ConfidenceWeightedVote         = aggregation.ConfidenceWeightedVote
	WeightedVote                   = aggregation.WeightedVote
	ConfidenceWeightedWeightedVote = aggregation.ConfidenceWeightedWeightedVote
)

// Classification.
var (
	ThresholdClassifier = core.ThresholdClassifier
)

// Persistence.
var (
	NewJSONLRecorder = persist.NewJSONLRecorder
	SaveSummary      = persist.SaveSummary
	LoadSummary      = persist.LoadSummary
	SaveSummaryFile  = persist.SaveSummaryFile
	LoadSummaryFile  = persist.LoadSummaryFile
)

// Re-scoring a saved summary (pure, zero model calls).
var (
	ReclassifySummary = core.ReclassifySummary
)
