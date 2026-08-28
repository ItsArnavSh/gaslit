package gaslit

import (
	"context"
	"fmt"
)

// DebateMany runs a debate for each (question, options) pair in order, reusing
// the panel's conversation state so agents carry context across questions.
func (g *Gaslit) DebateMany(ctx context.Context, questions []string, optionsList [][]string, opts ...CallOption) ([]*FullDebateSummary, error) {
	if len(questions) != len(optionsList) {
		return nil, fmt.Errorf("questions and optionsList must have equal length: got %d and %d", len(questions), len(optionsList))
	}

	summaries := make([]*FullDebateSummary, 0, len(questions))
	for i, q := range questions {
		s, err := g.Debate(ctx, q, optionsList[i], opts...)
		if err != nil {
			return nil, fmt.Errorf("debate %d failed: %w", i, err)
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}
