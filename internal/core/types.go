// Package core holds the shared data types of the gaslit engine. It is a leaf
// package: every other internal package imports it, and it imports nothing
// internal. The module root re-exports these types under the public `gaslit`
// package via type aliases.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
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

// Resolve fills zero fields with the default values.
func (t Thresholds) Resolve() Thresholds {
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

// QuadrantState is the 2D classification of an agent based on its confidence
// (x-axis) and semantic consistency (y-axis) under cross-examination.
type QuadrantState string

const (
	// QuadrantPeak indicates high confidence and high consistency.
	QuadrantPeak QuadrantState = "PEAK (Grounded Knowledge)"
	// QuadrantHallucinating indicates high confidence with fabricated contradictions.
	QuadrantHallucinating QuadrantState = "HALLUCINATING / MALICIOUS"
	// QuadrantKBDeficit indicates consistent but low-confidence answers.
	QuadrantKBDeficit QuadrantState = "KB_DEFICIT (Incomplete Info)"
	// QuadrantWrong indicates low confidence and inconsistent internal logic.
	QuadrantWrong QuadrantState = "WRONG / UNCERTAIN"
)

// DiagnosticResult is the outcome of grilling a single agent: where it lands on
// the confidence/consistency grid and what action is recommended.
type DiagnosticResult struct {
	AgentName        string        `json:"agent_name"`
	ConfidenceScore  float64       `json:"confidence_score"`  // X-axis (0.0 -> 1.0)
	ConsistencyScore float64       `json:"consistency_score"` // Y-axis (0.0 -> 1.0)
	Quadrant         QuadrantState `json:"quadrant"`
	InitialReasonR1  string        `json:"r1_initial_reasoning"`
	CounterReasonR2  string        `json:"r2_counter_reasoning"`
	Recommendation   string        `json:"recommendation"`
}

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

// DriftVerdict describes whether agents changed their positions over the course
// of a debate, which agents moved, and who drove the shift.
type DriftVerdict struct {
	// HasDrift reports whether any agent flipped or significantly eroded.
	HasDrift bool `json:"has_drift"`
	// DriftedAgents lists the names of agents that drifted.
	DriftedAgents []string `json:"drifted_agents"`
	// PrimaryDriver is the agent (or the supervisor) responsible for the shift.
	PrimaryDriver string `json:"primary_driver"`
	// DriverImpact is a human-readable description of who swayed whom.
	DriverImpact string `json:"driver_impact"`
	// AgentShifts maps each agent name to its measured shift.
	AgentShifts map[string]float64 `json:"agent_shifts"`
}

// EventType identifies a recorded pipeline event.
type EventType string

const (
	EventPhaseStart EventType = "phase_start"
	EventPhaseEnd   EventType = "phase_end"
	EventBaseline   EventType = "baseline"
	EventCrossExam  EventType = "cross_exam"
	EventEval       EventType = "evaluation"
	EventDrift      EventType = "drift"
	EventDiagnostic EventType = "diagnostic"
	EventSummary    EventType = "summary"
	EventDone       EventType = "done"
	EventError      EventType = "error"
)

// Event is a JSON-serializable record of a pipeline milestone, suitable for
// persistence or replay.
type Event struct {
	Time  time.Time       `json:"time"`
	Type  EventType       `json:"type"`
	Agent string          `json:"agent,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Recorder receives an Event stream for persistence.
type Recorder interface {
	Record(ctx context.Context, ev Event) error
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

// InstrumentationStats is a point-in-time snapshot of engine usage.
type InstrumentationStats struct {
	TotalCalls    int64
	Tokens        int64
	AgentCalls    map[string]int64
	PhaseDuration map[string][]time.Duration
}

// StreamEvent is a single milestone emitted by the streaming API.
type StreamEvent struct {
	Type       EventType         `json:"type"`
	Phase      Phase             `json:"phase,omitempty"`
	Agent      string            `json:"agent,omitempty"`
	Prompt     string            `json:"prompt,omitempty"`
	Answer     *Answer           `json:"answer,omitempty"`
	Verdict    *DriftVerdict     `json:"verdict,omitempty"`
	Diagnostic *DiagnosticResult `json:"diagnostic,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	Error      error             `json:"-"`
}
