package gaslit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
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

// classifyQuadrant maps confidence and consistency scores onto the 2D quadrant
// grid and returns the corresponding quadrant and recommendation.
func classifyQuadrant(confScore, consistencyScore float64) (QuadrantState, string) {
	const confThreshold = 0.55
	const consistThreshold = 0.60

	switch {
	case confScore >= confThreshold && consistencyScore >= consistThreshold:
		return QuadrantPeak, "No change needed. Agent demonstrates high factual resilience."

	case confScore >= confThreshold && consistencyScore < consistThreshold:
		return QuadrantHallucinating, "Isolate or downweight agent. Displays high confidence but fabricated mutually exclusive premises."

	case confScore < confThreshold && consistencyScore >= consistThreshold:
		return QuadrantKBDeficit, "Update knowledge base (RAG). Logic is consistent but lacks definitive evidence."

	default:
		return QuadrantWrong, "Agent is guessing with low conviction and inconsistent internal logic."
	}
}

// grillAgent evaluates an agent's confidence vs. consistency under cross-examination.
func (g *Gaslit) grillAgent(
	ctx context.Context,
	agentName string,
	confidences []float64,
	initialAnswer string,
	initialReasonR1 string,
	question string,
) (*DiagnosticResult, error) {
	// 1. Calculate Mean/Peak Confidence (X-Axis)
	if len(confidences) == 0 {
		return nil, fmt.Errorf("confidences array cannot be empty")
	}
	var confSum float64
	for _, c := range confidences {
		confSum += c
	}
	confScore := confSum / float64(len(confidences))

	// 2. Grilling Probe: Force the agent to defend the counter-factual / opposite stance (R2)
	grillPrompt := fmt.Sprintf(
		"CRITICAL INVERSION PROBE:\nQuestion: %q\nYour baseline claim was: %q\n\n"+
			"Assume your initial claim is strictly FALSE. Provide the strongest factual defense "+
			"and historical/logical evidence supporting the alternative position. "+
			"Do not hedge or apologize. Provide direct factual reasoning.",
		question, initialAnswer,
	)

	time.Sleep(groqPacingDelay)
	st := g.states[agentName]
	st.history = append(st.history, schema.UserMessage(grillPrompt))

	iter := st.agent.Run(ctx, &adk.TypedAgentInput[*schema.Message]{
		Messages: st.history,
	})
	resp, err := consumeAgentOutput(iter)
	if err != nil {
		return nil, fmt.Errorf("grilling probe failed on %s: %w", agentName, err)
	}
	st.history = append(st.history, sanitizeHistoryMessage(resp))
	counterReasonR2 := resp.Content

	// 3. Supervisor Evaluates Contradiction between R1 and R2 (Y-Axis)
	consistencyScore, err := g.evaluateSemanticConsistency(ctx, question, initialReasonR1, counterReasonR2)
	if err != nil {
		return nil, fmt.Errorf("consistency evaluation failed: %w", err)
	}

	// 4. Map into 2D Quadrant
	quadrant, recommendation := classifyQuadrant(confScore, consistencyScore)

	return &DiagnosticResult{
		AgentName:        agentName,
		ConfidenceScore:  confScore,
		ConsistencyScore: consistencyScore,
		Quadrant:         quadrant,
		InitialReasonR1:  initialReasonR1,
		CounterReasonR2:  counterReasonR2,
		Recommendation:   recommendation,
	}, nil
}

// evaluateSemanticConsistency prompts the supervisor to run an NLI check between R1 and R2
func (g *Gaslit) evaluateSemanticConsistency(ctx context.Context, question, r1, r2 string) (float64, error) {
	evalPrompt := fmt.Sprintf(`You are an impartial logic and entailment evaluator.
Analyze the following two arguments given by the same agent on the question: %q

Argument 1 (Defending Baseline):
%s

Argument 2 (Defending Counter-Position):
%s

Task:
1. Check if the agent fabricated mutually exclusive facts, contradictory dates, or false assertions between R1 and R2.
2. If the agent cleanly argued alternate perspectives without inventing contradictory "facts", score consistency high (0.8 - 1.0).
3. If the agent directly contradicted its own factual claims, score consistency low (0.0 - 0.4).

Respond ONLY in valid JSON matching this schema:
{
  "consistency_score": float (between 0.0 and 1.0),
  "contradictions": ["list of specific factual contradictions, if any"]
}`, question, r1, r2)

	time.Sleep(groqPacingDelay)
	supIter := g.supervisor.Run(ctx, &adk.TypedAgentInput[*schema.Message]{
		Messages: []*schema.Message{schema.UserMessage(evalPrompt)},
	})
	supResp, err := consumeAgentOutput(supIter)
	if err != nil {
		return 0.5, err
	}

	var parsed contradictionEval
	raw := strings.TrimSpace(supResp.Content)
	// Handle markdown wrappers if present
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")

	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return 0.5, nil // Default fallback if parsing encounters malformed formatting
	}

	return parsed.ConsistencyScore, nil
}
