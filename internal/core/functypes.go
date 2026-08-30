package core

import (
	"context"
	"strings"
)

// DriftMetric computes the distribution shift between an agent's before and
// after confidence distributions over the given option labels.
type DriftMetric func(labels []string, before, after *Answer) float64

// AttributionFn determines who drove the primary shift. It mutates the verdict
// with PrimaryDriver and DriverImpact.
type AttributionFn func(res *PromptResult, labels []string, verdict *DriftVerdict)

// AggregationFn derives the winning option from the panel's final positions.
// It returns the winning option label, or "" when there is no clear winner.
type AggregationFn func(after map[string]*Answer, labels []string, weights map[string]float64) string

// Classifier maps confidence and consistency scores onto the 2D quadrant grid.
type Classifier func(confScore, consistencyScore float64) (QuadrantState, string)

// ConsistencyScorer evaluates whether two arguments from the same agent
// contradict each other. Scores range 0.0 (contradicts itself) to 1.0 (clean).
type ConsistencyScorer interface {
	Score(ctx context.Context, question, r1, r2 string) (float64, error)
}

// ThresholdClassifier is the default classifier, using the configured
// confidence/consistency thresholds.
func ThresholdClassifier(confScore, consistencyScore float64) (QuadrantState, string) {
	thr := Thresholds{}.Resolve()
	return ClassifyWith(confScore, consistencyScore, thr)
}

// ClassifyWith maps (confidence, consistency) onto the quadrant grid.
func ClassifyWith(confScore, consistencyScore float64, thr Thresholds) (QuadrantState, string) {
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

// ReclassifySummary recomputes every diagnostic's quadrant and recommendation
// from its stored confidence and consistency scores. It makes no model calls,
// so it is the cheapest way to iterate on thresholds or quadrant-tallying
// logic against a saved FullDebateSummary.
func ReclassifySummary(s *FullDebateSummary, thr Thresholds) *FullDebateSummary {
	if s == nil {
		return nil
	}
	for _, d := range s.Diagnostics {
		if d == nil {
			continue
		}
		q, rec := ClassifyWith(d.ConfidenceScore, d.ConsistencyScore, thr)
		d.Quadrant = q
		d.Recommendation = rec
	}
	return s
}

// Clamp01 bounds a float to [0.0, 1.0].
func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Tokenize lowercases s and splits it into a set of cleaned word tokens.
func Tokenize(s string) map[string]bool {
	tokens := make(map[string]bool)
	for _, f := range strings.Fields(strings.ToLower(s)) {
		f = strings.Trim(f, " .,;:!?\"'()[]{}<>")
		if f != "" {
			tokens[f] = true
		}
	}
	return tokens
}

// ExtractJSONObject finds the outermost balanced JSON object in raw output.
func ExtractJSONObject(raw string) (string, error) {
	end := strings.LastIndex(raw, "}")
	if end == -1 {
		return "", &JSONError{msg: "no closing brace found in response"}
	}

	depth := 0
	start := -1
	for i := end; i >= 0; i-- {
		switch raw[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				start = i
			}
		}
		if start != -1 {
			break
		}
	}
	if start == -1 {
		return "", &JSONError{msg: "no matching opening brace found for JSON object"}
	}
	return raw[start : end+1], nil
}

// JSONError is a sentinel for malformed JSON extraction.
type JSONError struct{ msg string }

func (e *JSONError) Error() string { return e.msg }
