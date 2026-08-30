package engine

import (
	"context"
	"fmt"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

// Rescore re-runs the configured consistency scorer over each diagnostic's
// stored arguments (R1 vs R2) and then reclassifies it. It does not re-run the
// baseline or debate — only the scoring/quadrant step — so it is the right
// tool for iterating on the rubric against a saved FullDebateSummary without
// burning fresh model calls on the debate itself.
//
// The active scorer comes from the per-call options (see
// WithConsistencyScorerCall) or the engine configuration; budget, pacing, and
// call accounting apply to model-backed scorers.
func (g *Gaslit) Rescore(ctx context.Context, s *core.FullDebateSummary, opts ...core.CallOption) (*core.FullDebateSummary, error) {
	if s == nil {
		return nil, nil
	}
	co := g.resolveCall(opts...)
	for name, d := range s.Diagnostics {
		if d == nil {
			continue
		}
		consistency, err := g.score(ctx, co, s.Question, d.InitialReasonR1, d.CounterReasonR2)
		if err != nil {
			return nil, fmt.Errorf("rescore %s failed: %w", name, err)
		}
		d.ConsistencyScore = consistency
		q, rec := core.ClassifyWith(d.ConfidenceScore, consistency, co.Thresholds)
		d.Quadrant = q
		d.Recommendation = rec
	}
	return s, nil
}
