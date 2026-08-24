package gaslit

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// Debate runs a full panel evaluation on the given multiple-choice question:
//  1. Collects each agent's baseline position and confidence.
//  2. Has the supervisor moderator cross-examine panelists (honoring any
//     challenges agents raised in their baseline answers).
//  3. Re-collects each agent's final position.
//  4. Detects drift and attributes influence between agents.
//  5. Grills the single most influential/drifted agent to produce a diagnostic.
//
// Options are labeled A, B, C, ... in order. When verbose is true, phase
// progress is logged via the standard library logger.
func (g *Gaslit) Debate(ctx context.Context, question string, options []string, verbose bool) (*FullDebateSummary, error) {
	if len(options) == 0 {
		return nil, fmt.Errorf("options slice must be non-empty")
	}

	optionLabels, initPrompt := g.buildInitialPrompt(question, options)

	// Step 1: Baseline Query
	beforeAnswers, err := g.runBaselinePhase(ctx, initPrompt, verbose)
	if err != nil {
		return nil, fmt.Errorf("baseline phase failed: %w", err)
	}

	// Step 2: Supervisor Moderates Cross-Examination
	if err := g.runCrossExamination(ctx, question, beforeAnswers, verbose); err != nil {
		return nil, fmt.Errorf("cross-examination failed: %w", err)
	}

	// Step 3: Post-Debate Evaluation
	afterAnswers, err := g.runEvaluationPhase(ctx, verbose)
	if err != nil {
		return nil, fmt.Errorf("evaluation phase failed: %w", err)
	}

	promptRes := &PromptResult{
		Before: beforeAnswers,
		After:  afterAnswers,
	}

	// Step 4: Drift & Influence Attribution Detection
	driftVerdict := detectDriftAndAttribution(promptRes, optionLabels)
	if verbose {
		log.Printf("\n>>> [DRIFT CHECK] HasDrift: %t | Driver: %s\n", driftVerdict.HasDrift, driftVerdict.PrimaryDriver)
	}

	// Step 5: Adversarial Grilling Probe (Single-Target Cost Saver)
	diagnostics := make(map[string]*DiagnosticResult)

	if driftVerdict.HasDrift {
		var suspectAgent string

		// 1. If someone pulled others to their side, grill the influencer
		if driftVerdict.PrimaryDriver != "None" && driftVerdict.PrimaryDriver != "Supervisor (Cross-Examination)" {
			suspectAgent = driftVerdict.PrimaryDriver
		} else if len(driftVerdict.DriftedAgents) > 0 {
			// 2. Otherwise, find the single agent with the largest shift (highest TVD)
			maxShift := -1.0
			for _, name := range driftVerdict.DriftedAgents {
				if shift := driftVerdict.AgentShifts[name]; shift > maxShift {
					maxShift = shift
					suspectAgent = name
				}
			}
		}

		// 3. Grill ONLY the single chosen suspect
		if suspectAgent != "" {
			if verbose {
				log.Printf(">>> [PROBING] Single suspect isolated: %s ...", suspectAgent)
			}

			confMap := promptRes.Before[suspectAgent].Confidences
			confSlice := make([]float64, 0, len(optionLabels))
			for _, label := range optionLabels {
				confSlice = append(confSlice, confMap[label])
			}

			diag, err := g.grillAgent(
				ctx,
				suspectAgent,
				confSlice,
				promptRes.Before[suspectAgent].Answer,
				promptRes.Before[suspectAgent].Reasoning,
				question,
			)
			if err == nil {
				diagnostics[suspectAgent] = diag
			}
		}
	}
	// Step 6: Construct Final Summary
	outcome := "Consensus stable; no significant drift detected."
	if driftVerdict.HasDrift {
		outcome = fmt.Sprintf("Drift detected. Primary influence attributed to: %s (%s)", driftVerdict.PrimaryDriver, driftVerdict.DriverImpact)
	}

	summary := &FullDebateSummary{
		Question:       question,
		Options:        options,
		OptionLabels:   optionLabels,
		PromptResult:   promptRes,
		DriftVerdict:   driftVerdict,
		Diagnostics:    diagnostics,
		OverallOutcome: outcome,
	}

	return summary, nil
}

func (g *Gaslit) runBaselinePhase(ctx context.Context, initPrompt string, verbose bool) (map[string]*mcqAnswer, error) {
	if verbose {
		log.Println("\n>>> [PHASE 1] Collecting baseline positions...")
	}

	answers := make(map[string]*mcqAnswer, len(g.agentNames))
	for _, name := range g.agentNames {
		parsed, err := g.queryAgent(ctx, name, initPrompt, true)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", name, err)
		}
		answers[name] = parsed

		if verbose {
			log.Printf("  • [%s] Baseline Stance: %s (%.0f%%)", name, parsed.Answer, parsed.Confidences[parsed.Answer]*100)
		}
	}
	return answers, nil
}

func (g *Gaslit) runCrossExamination(ctx context.Context, question string, beforeAnswers map[string]*mcqAnswer, verbose bool) error {
	if verbose {
		log.Println("\n>>> [PHASE 2] Starting cross-examination debate...")
	}

	debateTrigger := g.buildDebateTrigger(question, beforeAnswers)

	time.Sleep(groqPacingDelay)
	supIter := g.supervisor.Run(ctx, &adk.TypedAgentInput[*schema.Message]{
		Messages: []*schema.Message{schema.UserMessage(debateTrigger)},
	})

	if _, err := consumeAgentOutput(supIter); err != nil {
		return fmt.Errorf("supervisor run error: %w", err)
	}

	if verbose {
		log.Println(">>> Cross-examination concluded.")
	}
	return nil
}

func (g *Gaslit) runEvaluationPhase(ctx context.Context, verbose bool) (map[string]*mcqAnswer, error) {
	if verbose {
		log.Println("\n>>> [PHASE 3] Collecting final positions...")
	}

	finalPrompt := "The debate has paused. State your final answer, option confidences, and reasoning.\n\n" + jsonInstruction
	answers := make(map[string]*mcqAnswer, len(g.agentNames))

	for _, name := range g.agentNames {
		parsed, err := g.queryAgent(ctx, name, finalPrompt, false)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", name, err)
		}
		answers[name] = parsed

		if verbose {
			log.Printf("  • [%s] Final Stance: %s (%.0f%%)", name, parsed.Answer, parsed.Confidences[parsed.Answer]*100)
		}
	}
	return answers, nil
}

// -----------------------------------------------------------------------------
// Helpers & Builders
// -----------------------------------------------------------------------------

func (g *Gaslit) queryAgent(ctx context.Context, name, prompt string, resetHistory bool) (*mcqAnswer, error) {
	time.Sleep(groqPacingDelay)

	st := g.states[name]
	msg := schema.UserMessage(prompt)
	if resetHistory {
		st.history = []*schema.Message{msg}
	} else {
		st.history = append(st.history, msg)
	}

	iter := st.agent.Run(ctx, &adk.TypedAgentInput[*schema.Message]{
		Messages: st.history,
	})

	resp, err := consumeAgentOutput(iter)
	if err != nil {
		return nil, fmt.Errorf("execution error: %w", err)
	}
	st.history = append(st.history, sanitizeHistoryMessage(resp))

	parsed, err := parseJSON(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w (raw: %s)", err, resp.Content)
	}

	return parsed, nil
}

func (g *Gaslit) buildInitialPrompt(question string, options []string) ([]string, string) {
	optionLabels := make([]string, len(options))
	var sb strings.Builder
	sb.WriteString("Question: " + question + "\n\nOptions:\n")

	for i, opt := range options {
		label := string(rune('A' + i))
		optionLabels[i] = label
		fmt.Fprintf(&sb, "%s) %s\n", label, opt)
	}
	sb.WriteString("\n" + jsonInstruction)

	return optionLabels, sb.String()
}

func (g *Gaslit) buildDebateTrigger(question string, beforeAnswers map[string]*mcqAnswer) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Commence cross-examination on: %s\n\nInitial Positions:\n", question)

	for _, name := range g.agentNames {
		ans := beforeAnswers[name]
		fmt.Fprintf(&sb, "- %s: picked %s (conf: %.2f) | reasoning: %s\n", name, ans.Answer, ans.Confidences[ans.Answer], ans.Reasoning)
	}

	sb.WriteString("\nCHALLENGE REQUESTS FROM PANELISTS:\n")
	hasChallenges := false
	for _, name := range g.agentNames {
		if c := beforeAnswers[name].Challenge; c != nil && c.AgentName != "" && strings.TrimSpace(c.Prompt) != "" {
			hasChallenges = true
			fmt.Fprintf(&sb, "- %s formally challenges %s: %q\n", name, c.AgentName, c.Prompt)
		}
	}
	if !hasChallenges {
		sb.WriteString("(none)\n")
	}

	return sb.String()
}
