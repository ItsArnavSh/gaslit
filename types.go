// Package gaslit stress-tests AI agents by staging a supervised courtroom
// debate. The public API of this package is a thin facade over the
// implementation packages under internal/: every type here is an alias and
// every function/variable is a re-export, so the API surface is identical to a
// single flat package while the code stays organized by feature.
package gaslit

import (
	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/engine"
	"github.com/ItsArnavSh/gaslit/internal/persist"
	"github.com/ItsArnavSh/gaslit/internal/scoring"
)

// Types.
type (
	Phase                = core.Phase
	Thresholds           = core.Thresholds
	CostBudget           = core.CostBudget
	AgentConfig          = core.AgentConfig
	Config               = core.Config
	Protocol             = core.Protocol
	GrillStrategy        = core.GrillStrategy
	Option               = core.Option
	CallOption           = core.CallOption
	CallOptions          = core.CallOptions
	QuadrantState        = core.QuadrantState
	DiagnosticResult     = core.DiagnosticResult
	Classifier           = core.Classifier
	ConsistencyScorer    = core.ConsistencyScorer
	LLMScorer            = scoring.LLMScorer
	LexicalScorer        = scoring.LexicalScorer
	Answer               = core.Answer
	PromptResult         = core.PromptResult
	FullDebateSummary    = core.FullDebateSummary
	Gaslit               = engine.Gaslit
	DriftMetric          = core.DriftMetric
	AttributionFn        = core.AttributionFn
	DriftVerdict         = core.DriftVerdict
	AggregationFn        = core.AggregationFn
	EventType            = core.EventType
	Event                = core.Event
	Recorder             = core.Recorder
	JSONLRecorder        = persist.JSONLRecorder
	AnswerSchema         = core.AnswerSchema
	MCQSchema            = core.MCQSchema
	OpenEndedSchema      = core.OpenEndedSchema
	PromptTemplates      = core.PromptTemplates
	Hooks                = core.Hooks
	BudgetError          = core.BudgetError
	InstrumentationStats = core.InstrumentationStats
	StreamEvent          = core.StreamEvent
)
