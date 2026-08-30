// Package scoring implements the consistency scorers used to detect whether an
// agent's arguments contradict each other under cross-examination.
package scoring

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type contradictionEval struct {
	ConsistencyScore float64  `json:"consistency_score"` // 1.0 = no contradictions, 0.0 = severe contradiction
	Contradictions   []string `json:"contradictions"`
}

// LLMScorer implements core.ConsistencyScorer by having a model run a logical
// entailment / contradiction check between two arguments.
type LLMScorer struct {
	Model  model.BaseModel[*schema.Message]
	Logger *slog.Logger
	// NLIProbe builds the evaluation prompt. When nil, the built-in template
	// is used.
	NLIProbe func(question, r1, r2 string) string
}

// Score runs the entailment probe and parses the consistency_score. It falls
// back to a neutral 0.5 on any parse failure.
func (s *LLMScorer) Score(ctx context.Context, question, r1, r2 string) (float64, error) {
	prompt := s.NLIProbe
	if prompt == nil {
		prompt = core.DefaultNLIProbe
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

// LexicalScorer implements core.ConsistencyScorer deterministically with zero
// extra model calls: it returns the Jaccard similarity of the two arguments'
// word sets. It is best used as a cheap approximation when an LLM evaluator is
// not available, e.g. while debugging the pipeline.
type LexicalScorer struct{}

// Score returns the lexical overlap between the two arguments as a consistency
// proxy.
func (s *LexicalScorer) Score(ctx context.Context, question, r1, r2 string) (float64, error) {
	if r1 == "" || r2 == "" {
		return 0.5, nil
	}
	u := core.Tokenize(r1)
	v := core.Tokenize(r2)
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
	return core.Clamp01(float64(inter) / float64(union)), nil
}
