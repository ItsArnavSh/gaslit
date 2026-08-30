package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/drift"
)

// Debate runs a full panel evaluation on the given question:
//  1. Collects each agent's baseline position and confidence.
//  2. Cross-examines panelists (moderator-led or protocol-driven).
//  3. Re-collects each agent's final position.
//  4. Detects drift and attributes influence between agents.
//  5. Grills the agents selected by the active GrillStrategy.
//
// Options are labeled A, B, C, ... in order (or via WithOptionLabelsCall). Pass
// an empty options slice for open-ended questions. When verbose is true, phase
// progress is logged through the configured logger.
func (g *Gaslit) Debate(ctx context.Context, question string, options []string, opts ...core.CallOption) (*core.FullDebateSummary, error) {
	co := g.resolveCall(opts...)
	ctx, cancel := g.withBudgetContext(ctx, co.Budget)
	defer cancel()

	labels, _, _ := g.buildInitialPrompt(question, options, co)

	before, err := g.Baseline(ctx, question, options, opts...)
	if err != nil {
		return nil, fmt.Errorf("baseline phase failed: %w", err)
	}

	current := before
	var after map[string]*core.Answer
	for round := 0; round < co.MaxRounds; round++ {
		if err := g.CrossExamine(ctx, question, current, opts...); err != nil {
			return nil, fmt.Errorf("cross-examination failed (round %d): %w", round+1, err)
		}
		after, err = g.Evaluate(ctx, options, opts...)
		if err != nil {
			return nil, fmt.Errorf("evaluation phase failed (round %d): %w", round+1, err)
		}
		current = after
	}

	promptRes := &core.PromptResult{Before: before, After: after}
	verdict := g.analyze(ctx, promptRes, labels, co)

	diagnostics := g.grillPhase(ctx, question, promptRes, verdict, labels, co)

	weights := make(map[string]float64, len(g.cfg.AgentOverrides))
	for name, ac := range g.cfg.AgentOverrides {
		weights[name] = ac.Weight
	}
	winner := g.cfg.AggregationFn(after, labels, weights)

	outcome := "Consensus stable; no significant drift detected."
	if verdict.HasDrift {
		outcome = fmt.Sprintf("Drift detected. Primary influence attributed to: %s (%s)", verdict.PrimaryDriver, verdict.DriverImpact)
	}

	summary := &core.FullDebateSummary{
		Question:       question,
		Options:        options,
		OptionLabels:   labels,
		PromptResult:   promptRes,
		DriftVerdict:   verdict,
		Diagnostics:    diagnostics,
		Winner:         winner,
		OverallOutcome: outcome,
		Metadata:       co.Metadata,
	}
	g.record(core.EventSummary, "", summary)
	return summary, nil
}

// DebateN runs a Debate with a specific number of (cross-examine → evaluate)
// rounds, overriding the engine's MaxRounds for this call.
func (g *Gaslit) DebateN(ctx context.Context, question string, options []string, rounds int, opts ...core.CallOption) (*core.FullDebateSummary, error) {
	return g.Debate(ctx, question, options, append(opts, core.WithMaxRoundsCall(rounds))...)
}

// Baseline collects each agent's initial position on the question.
func (g *Gaslit) Baseline(ctx context.Context, question string, options []string, opts ...core.CallOption) (map[string]*core.Answer, error) {
	co := g.resolveCall(opts...)
	hk := g.hooks(co)
	g.phaseStart(ctx, core.PhaseBaseline, hk)
	defer g.phaseEnd(ctx, core.PhaseBaseline, hk)

	_, initPrompt, as := g.buildInitialPrompt(question, options, co)
	g.logf(co, ">>> [PHASE 1] Collecting baseline positions...")

	answers := make(map[string]*core.Answer, len(g.agentNames))
	var mu sync.Mutex
	err := g.forEachAgent(ctx, g.agentNames, func(ctx context.Context, name string) error {
		if g.stateSkipped(name, core.PhaseBaseline) {
			return nil
		}
		parsed, err := g.queryAgent(ctx, name, g.redact(initPrompt), true, as)
		if err != nil {
			return err
		}
		mu.Lock()
		answers[name] = parsed
		mu.Unlock()

		if hk.OnBaseline != nil {
			hk.OnBaseline(ctx, name, parsed)
		}
		g.record(core.EventBaseline, name, parsed)
		if co.Verbose {
			g.logf(co, "  • [%s] Baseline Stance: %s (%.0f%%)", name, parsed.Answer, parsed.Confidences[parsed.Answer]*100)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return answers, nil
}

// CrossExamine runs one cross-examination round against the given positions.
func (g *Gaslit) CrossExamine(ctx context.Context, question string, before map[string]*core.Answer, opts ...core.CallOption) error {
	co := g.resolveCall(opts...)
	return g.runCrossExamination(ctx, question, before, co)
}

// Evaluate re-collects each agent's final position after cross-examination.
func (g *Gaslit) Evaluate(ctx context.Context, options []string, opts ...core.CallOption) (map[string]*core.Answer, error) {
	co := g.resolveCall(opts...)
	hk := g.hooks(co)
	g.phaseStart(ctx, core.PhaseEvaluation, hk)
	defer g.phaseEnd(ctx, core.PhaseEvaluation, hk)

	as := g.schemaFor(options)
	finalPrompt := g.cfg.PromptTemplates.FinalEvalPrompt + "\n" + as.Instruction()
	g.logf(co, ">>> [PHASE 3] Collecting final positions...")

	answers := make(map[string]*core.Answer, len(g.agentNames))
	var mu sync.Mutex
	err := g.forEachAgent(ctx, g.agentNames, func(ctx context.Context, name string) error {
		if g.stateSkipped(name, core.PhaseEvaluation) {
			return nil
		}
		parsed, err := g.queryAgent(ctx, name, g.redact(finalPrompt), false, as)
		if err != nil {
			return err
		}
		mu.Lock()
		answers[name] = parsed
		mu.Unlock()

		if hk.OnEval != nil {
			hk.OnEval(ctx, name, parsed)
		}
		g.record(core.EventEval, name, parsed)
		if co.Verbose {
			g.logf(co, "  • [%s] Final Stance: %s (%.0f%%)", name, parsed.Answer, parsed.Confidences[parsed.Answer]*100)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return answers, nil
}

// DriftAnalysis computes the drift verdict between before and after positions
// using the engine's configured metric, thresholds, and attribution strategy.
func (g *Gaslit) DriftAnalysis(before, after map[string]*core.Answer, options []string, opts ...core.CallOption) core.DriftVerdict {
	co := g.resolveCall(opts...)
	labels, _, _ := g.buildInitialPrompt("", options, co)
	return drift.Analyze(&core.PromptResult{Before: before, After: after}, labels, co.Thresholds, g.cfg.DriftMetric, g.cfg.AttributionFn)
}

// analyze wraps the pure drift computation with hooks, instrumentation, and
// recording.
func (g *Gaslit) analyze(ctx context.Context, res *core.PromptResult, labels []string, co core.CallOptions) core.DriftVerdict {
	hk := g.hooks(co)
	g.phaseStart(ctx, core.PhaseDriftAnalysis, hk)

	verdict := drift.Analyze(res, labels, co.Thresholds, g.cfg.DriftMetric, g.cfg.AttributionFn)

	if hk.OnDrift != nil {
		hk.OnDrift(ctx, &verdict)
	}
	g.record(core.EventDrift, "", &verdict)
	g.logf(co, ">>> [DRIFT CHECK] HasDrift: %t | Driver: %s", verdict.HasDrift, verdict.PrimaryDriver)

	g.phaseEnd(ctx, core.PhaseDriftAnalysis, hk)
	return verdict
}

// buildInitialPrompt builds the option labels, the baseline prompt, and the
// active answer schema for a question.
func (g *Gaslit) buildInitialPrompt(question string, options []string, co core.CallOptions) (labels []string, prompt string, as core.AnswerSchema) {
	as = g.schemaFor(options)
	if len(options) == 0 {
		return nil, g.cfg.PromptTemplates.InitialPrompt(question, nil, nil, as.Instruction()), as
	}

	labels = co.OptionLabels
	if len(labels) != len(options) {
		labels = autoLabels(len(options))
	}
	return labels, g.cfg.PromptTemplates.InitialPrompt(question, options, labels, as.Instruction()), as
}

func autoLabels(n int) []string {
	labels := make([]string, n)
	for i := range labels {
		labels[i] = string(rune('A' + i))
	}
	return labels
}

func (g *Gaslit) stateSkipped(name string, phase core.Phase) bool {
	st := g.states[name]
	return st != nil && st.cfg.SkipPhases != nil && st.cfg.SkipPhases[phase]
}

func (g *Gaslit) phaseStart(ctx context.Context, phase core.Phase, hk core.Hooks) {
	if hk.OnPhaseStart != nil {
		hk.OnPhaseStart(ctx, phase)
	}
	g.instr.phaseStart(phase)
}

func (g *Gaslit) phaseEnd(ctx context.Context, phase core.Phase, hk core.Hooks) {
	if hk.OnPhaseEnd != nil {
		hk.OnPhaseEnd(ctx, phase)
	}
	g.instr.phaseEnd(phase)
}
