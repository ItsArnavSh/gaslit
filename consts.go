package gaslit

import "github.com/ItsArnavSh/gaslit/internal/core"

// Phases.
const (
	PhaseBaseline         = core.PhaseBaseline
	PhaseCrossExamination = core.PhaseCrossExamination
	PhaseEvaluation       = core.PhaseEvaluation
	PhaseDriftAnalysis    = core.PhaseDriftAnalysis
	PhaseGrilling         = core.PhaseGrilling
)

// Protocols.
const (
	ProtocolSupervised       = core.ProtocolSupervised
	ProtocolAdversarialPairs = core.ProtocolAdversarialPairs
	ProtocolRoundRobin       = core.ProtocolRoundRobin
	ProtocolDevilsAdvocate   = core.ProtocolDevilsAdvocate
)

// Grill strategies.
const (
	GrillNone          = core.GrillNone
	GrillSingleSuspect = core.GrillSingleSuspect
	GrillInfluencer    = core.GrillInfluencer
	GrillAllDrifted    = core.GrillAllDrifted
	GrillTopN          = core.GrillTopN
)

// Quadrants.
const (
	QuadrantPeak          = core.QuadrantPeak
	QuadrantHallucinating = core.QuadrantHallucinating
	QuadrantKBDeficit     = core.QuadrantKBDeficit
	QuadrantWrong         = core.QuadrantWrong
)

// Event types.
const (
	EventPhaseStart = core.EventPhaseStart
	EventPhaseEnd   = core.EventPhaseEnd
	EventBaseline   = core.EventBaseline
	EventCrossExam  = core.EventCrossExam
	EventEval       = core.EventEval
	EventDrift      = core.EventDrift
	EventDiagnostic = core.EventDiagnostic
	EventSummary    = core.EventSummary
	EventDone       = core.EventDone
	EventError      = core.EventError
)

// Schema contract text.
const (
	DefaultJSONInstruction = core.DefaultJSONInstruction
	OpenEndedInstruction   = core.OpenEndedInstruction
)
