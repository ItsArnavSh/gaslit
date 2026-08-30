package engine

import (
	"context"
	"fmt"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

// directedChallenge is a single cross-examination instruction targeting one
// panelist on behalf of another.
type directedChallenge struct {
	Target string
	From   string
	Prompt string
}

// runCrossExamination executes the configured protocol for one cross-examination
// round against the given positions.
func (g *Gaslit) runCrossExamination(ctx context.Context, question string, before map[string]*core.Answer, co core.CallOptions) error {
	hk := g.hooks(co)
	g.phaseStart(ctx, core.PhaseCrossExamination, hk)
	defer g.phaseEnd(ctx, core.PhaseCrossExamination, hk)

	switch co.Protocol {
	case core.ProtocolAdversarialPairs, core.ProtocolRoundRobin, core.ProtocolDevilsAdvocate:
		challenges := planChallenges(co.Protocol, g.agentNames, before, g.cfg.PromptTemplates.ChallengePrompt)
		for _, ch := range challenges {
			st, ok := g.states[ch.Target]
			if !ok {
				continue
			}
			if hk.OnCrossExam != nil {
				hk.OnCrossExam(ctx, ch.Target, ch.Prompt)
			}
			g.record(core.EventCrossExam, ch.Target, map[string]string{"from": ch.From, "prompt": ch.Prompt})
			g.logf(co, ">>> [CROSS-EXAM] %s → %s", ch.From, ch.Target)

			userMsg := fmt.Sprintf(g.cfg.PromptTemplates.CrossExamineLead, g.redact(ch.Prompt), g.cfg.AnswerSchema.Instruction())
			if _, err := g.sendToAgent(ctx, st, userMsg); err != nil {
				return fmt.Errorf("cross-examine %s: %w", ch.Target, err)
			}
		}
		return nil

	default: // ProtocolSupervised
		g.logf(co, ">>> Starting supervised cross-examination...")
		trigger := g.cfg.PromptTemplates.DebateTrigger(question, g.agentNames, before)
		if _, err := g.runSupervisor(ctx, trigger); err != nil {
			return fmt.Errorf("supervisor run error: %w", err)
		}
		g.logf(co, ">>> Cross-examination concluded.")
		return nil
	}
}

// planChallenges computes the ordered directed challenges for a protocol. It is
// pure (no agent or model calls) so it can be unit tested.
func planChallenges(p core.Protocol, names []string, before map[string]*core.Answer, challengePrompt func(from, claim, reasoning string) string) []directedChallenge {
	var out []directedChallenge

	switch p {
	case core.ProtocolAdversarialPairs:
		for _, pair := range pairUp(names) {
			a, b := pair[0], pair[1]
			ab := before[a]
			ba := before[b]
			out = append(out,
				directedChallenge{Target: b, From: a, Prompt: challengePrompt(a, ab.Answer, ab.Reasoning)},
				directedChallenge{Target: a, From: b, Prompt: challengePrompt(b, ba.Answer, ba.Reasoning)},
			)
		}

	case core.ProtocolRoundRobin:
		for _, from := range names {
			for _, to := range names {
				if from == to {
					continue
				}
				f := before[from]
				out = append(out, directedChallenge{Target: to, From: from, Prompt: challengePrompt(from, f.Answer, f.Reasoning)})
			}
		}

	case core.ProtocolDevilsAdvocate:
		for _, name := range names {
			best, bestConf := "", -1.0
			for _, other := range names {
				if other == name {
					continue
				}
				o := before[other]
				if o.Answer == before[name].Answer {
					continue // prefer an opposing stance
				}
				if c := o.Confidences[o.Answer]; c > bestConf {
					bestConf = c
					best = other
				}
			}
			if best != "" {
				b := before[best]
				out = append(out, directedChallenge{Target: name, From: best, Prompt: challengePrompt(best, b.Answer, b.Reasoning)})
			} else {
				out = append(out, directedChallenge{
					Target: name,
					From:   "devil's advocate",
					Prompt: devilsAdvocateLocal(before[name]),
				})
			}
		}
	}

	return out
}

func pairUp(names []string) [][2]string {
	var pairs [][2]string
	for i := 0; i+1 < len(names); i += 2 {
		pairs = append(pairs, [2]string{names[i], names[i+1]})
	}
	if len(names)%2 == 1 && len(names) > 1 {
		pairs = append(pairs, [2]string{names[len(names)-1], names[0]})
	}
	return pairs
}

func devilsAdvocateLocal(ans *core.Answer) string {
	if len(ans.Confidences) == 0 {
		return "Consider the possibility that your position is wrong. Construct the strongest possible counter-argument and defend it."
	}
	worst, worstC := "", 1.0
	for opt, c := range ans.Confidences {
		if c < worstC {
			worstC = c
			worst = opt
		}
	}
	return fmt.Sprintf("Assume option %s is actually correct. Construct the strongest factual and logical defense of it, and challenge your own position with it.", worst)
}
