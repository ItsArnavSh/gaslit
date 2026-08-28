package gaslit

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

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

type contradictionEval struct {
	ConsistencyScore float64  `json:"consistency_score"` // 1.0 = no contradictions, 0.0 = severe contradiction
	Contradictions   []string `json:"contradictions"`
}

// Classifier maps confidence and consistency scores onto the 2D quadrant grid.
type Classifier func(confScore, consistencyScore float64) (QuadrantState, string)

// ThresholdClassifier is the default classifier, using the configured
// confidence/consistency thresholds.
func ThresholdClassifier(confScore, consistencyScore float64) (QuadrantState, string) {
	thr := Thresholds{}.resolve() // defaults; engine overrides via cfg
	return classifyWith(confScore, consistencyScore, thr)
}

func classifyWith(confScore, consistencyScore float64, thr Thresholds) (QuadrantState, string) {
	switch {
	case confScore >= thr.Confidence && consistencyScore >= thr.Consistency:
		return QuadrantPeak, "No change needed. Agent demonstrates high factual resilience."
	case confScore >= thr.Confidence && consistencyScore < thr.Consistency:
		return QuadrantHallucinating, "Isolate or downweight agent. Displays high confidence but fabricated mutually exclusive premises."
	case confScore < thr.Confidence && consistencyScore >= thr.Consistency:
		return QuadrantKBDeficit, "Update knowledge base (RAG). Logic is consistent but lacks definitive evidence."
	default:
		return QuadrantWrong, "Agent is guessing with low conviction and inconsistent internal logic."
	}
}

// ConsistencyScorer evaluates whether two arguments from the same agent
// contradict each other. Scores range 0.0 (contradicts itself) to 1.0 (clean).
type ConsistencyScorer interface {
	Score(ctx context.Context, question, r1, r2 string) (float64, error)
}

// LLMScorer implements ConsistencyScorer by having a model run a logical
// entailment / contradiction check between two arguments.
type LLMScorer struct {
	Model  model.BaseModel[*schema.Message]
	Logger *slog.Logger
	// NLIProbe builds the evaluation prompt. When nil, the engine's configured
	// prompt template is used.
	NLIProbe func(question, r1, r2 string) string
}

// Score runs the entailment probe and parses the consistency_score. It falls
// back to a neutral 0.5 on any parse failure.
func (s *LLMScorer) Score(ctx context.Context, question, r1, r2 string) (float64, error) {
	prompt := s.NLIProbe
	if prompt == nil {
		prompt = defaultNLIProbeBuilder
	}
	userMsg := schema.UserMessage(prompt(question, r1, r2))
	resp, err := s.Model.Generate(ctx, []*schema.Message{userMsg})
	if err != nil {
		return 0.5, err
	}

	var parsed contradictionEval
	raw := strings.TrimSpace(resp.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")

	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return 0.5, nil // Default fallback on malformed formatting
	}
	return parsed.ConsistencyScore, nil
}

// LexicalScorer implements ConsistencyScorer deterministically with zero extra
// model calls: it returns the Jaccard similarity of the two arguments' word
// sets. It is best used as a cheap approximation when an LLM evaluator is not
// available.
type LexicalScorer struct{}

// Score returns the lexical overlap between the two arguments as a consistency
// proxy.
func (s *LexicalScorer) Score(ctx context.Context, question, r1, r2 string) (float64, error) {
	if r1 == "" || r2 == "" {
		return 0.5, nil
	}
	u := tokenize(r1)
	v := tokenize(r2)
	if len(u) == 0 || len(v) == 0 {
		return 0.5, nil
	}
	inter, union := 0, len(u)
	for w := range u {
		if v[w] {
			inter++
		}
	}
	for w := range v {
		if !u[w] {
			union++
		}
	}
	if union == 0 {
		return 0.5, nil
	}
	return clamp01(float64(inter) / float64(union)), nil
}
