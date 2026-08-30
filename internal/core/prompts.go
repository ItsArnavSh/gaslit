package core

import (
	"fmt"
	"strings"
)

const defaultSupervisorSystem = `You are the Courtroom Debate Moderator.
Panelists: %s.

Rules:
1. NO FINAL VERDICT: Do NOT summarize, do NOT select a winner, and do NOT give a conclusion.
2. Cross-Examination: Take arguments made by one agent and challenge the others using 'cross_examine_agent'. Be pointed and adversarial — do not soften panelists' claims when you relay them.
3. Honor Nominations: If the prompt you receive lists panelist-submitted CHALLENGE REQUESTS, you MUST act on each one by calling 'cross_examine_agent' with that target and a sharpened version of the stated prompt, in addition to any challenges you initiate yourself.
4. Round Limit: Perform at most 3 to 4 challenges per agent.
5. Exit: Say "Debate ended." and stop once done.`

const defaultCrossExamineLead = "%s\n\nRe-evaluate your position under this cross-examination.\n%s"

const defaultFinalEvalPrompt = "The debate has paused. State your final answer, option confidences, and reasoning."

const defaultGrillProbe = `CRITICAL INVERSION PROBE:
Question: %q
Your baseline claim was: %q

Assume your initial claim is strictly FALSE. Provide the strongest factual defense and historical/logical evidence supporting the alternative position. Do not hedge or apologize. Provide direct factual reasoning.`

const defaultNLIProbe = `You are an impartial logic and entailment evaluator.
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
}`

const defaultConfidenceProbe = `How confident are you in the claim %q, on a scale of 0.0 to 1.0?
Respond with ONLY a valid JSON object: {"confidence": <float 0.0-1.0>}`

// PromptTemplates lets callers override any or all of the prompts the engine
// builds. Function fields default to the built-in builders; string fields
// default to the built-in templates. Zero fields are resolved by New.
type PromptTemplates struct {
	// SupervisorSystem is the moderator system prompt. %s is replaced with the
	// comma-joined panelist names.
	SupervisorSystem string
	// CrossExamineLead is formatted with (challenge prompt, schema
	// instruction) when the moderator or a protocol challenges an agent.
	CrossExamineLead string
	// FinalEvalPrompt precedes the schema instruction when re-collecting final
	// positions.
	FinalEvalPrompt string
	// InitialPrompt builds the baseline question prompt.
	InitialPrompt func(question string, options []string, labels []string, instr string) string
	// DebateTrigger builds the moderator's cross-examination briefing.
	DebateTrigger func(question string, names []string, before map[string]*Answer) string
	// GrillProbe builds the adversarial inversion probe.
	GrillProbe func(question, claim string) string
	// NLIProbe builds the consistency-evaluation prompt.
	NLIProbe func(question, r1, r2 string) string
	// ChallengePrompt builds a direct peer challenge used by the non-supervised
	// protocols.
	ChallengePrompt func(from, claim, reasoning string) string
	// ConfidenceProbe asks an agent to self-report confidence in a claim.
	ConfidenceProbe func(claim string) string
}

// Resolve fills zero fields with the built-in defaults.
func (t PromptTemplates) Resolve() PromptTemplates {
	if t.SupervisorSystem == "" {
		t.SupervisorSystem = defaultSupervisorSystem
	}
	if t.CrossExamineLead == "" {
		t.CrossExamineLead = defaultCrossExamineLead
	}
	if t.FinalEvalPrompt == "" {
		t.FinalEvalPrompt = defaultFinalEvalPrompt
	}
	if t.InitialPrompt == nil {
		t.InitialPrompt = defaultInitialPrompt
	}
	if t.DebateTrigger == nil {
		t.DebateTrigger = defaultDebateTrigger
	}
	if t.GrillProbe == nil {
		t.GrillProbe = defaultGrillProbeBuilder
	}
	if t.NLIProbe == nil {
		t.NLIProbe = defaultNLIProbeBuilder
	}
	if t.ChallengePrompt == nil {
		t.ChallengePrompt = defaultChallengePrompt
	}
	if t.ConfidenceProbe == nil {
		t.ConfidenceProbe = defaultConfidenceProbeBuilder
	}
	return t
}

// DefaultNLIProbe is the built-in entailment probe builder. It is used as the
// LLM scorer's fallback when no PromptTemplates.NLIProbe is wired.
func DefaultNLIProbe(question, r1, r2 string) string {
	return fmt.Sprintf(defaultNLIProbe, question, r1, r2)
}

func defaultInitialPrompt(question string, options []string, labels []string, instr string) string {
	var sb strings.Builder
	sb.WriteString("Question: " + question + "\n\nOptions:\n")
	for i, opt := range options {
		fmt.Fprintf(&sb, "%s) %s\n", labels[i], opt)
	}
	sb.WriteString("\n" + instr)
	return sb.String()
}

func defaultDebateTrigger(question string, names []string, before map[string]*Answer) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Commence cross-examination on: %s\n\nInitial Positions:\n", question)
	for _, name := range names {
		ans := before[name]
		fmt.Fprintf(&sb, "- %s: picked %s (conf: %.2f) | reasoning: %s\n", name, ans.Answer, ans.Confidences[ans.Answer], ans.Reasoning)
	}
	sb.WriteString("\nCHALLENGE REQUESTS FROM PANELISTS:\n")
	hasChallenges := false
	for _, name := range names {
		if c := before[name].Challenge; c != nil && c.AgentName != "" && strings.TrimSpace(c.Prompt) != "" {
			hasChallenges = true
			fmt.Fprintf(&sb, "- %s formally challenges %s: %q\n", name, c.AgentName, c.Prompt)
		}
	}
	if !hasChallenges {
		sb.WriteString("(none)\n")
	}
	return sb.String()
}

func defaultGrillProbeBuilder(question, claim string) string {
	return fmt.Sprintf(defaultGrillProbe, question, claim)
}

func defaultNLIProbeBuilder(question, r1, r2 string) string {
	return fmt.Sprintf(defaultNLIProbe, question, r1, r2)
}

func defaultChallengePrompt(from, claim, reasoning string) string {
	return fmt.Sprintf("Your colleague %s argues: %q (reasoning: %s). Re-evaluate your position under this cross-examination.", from, claim, reasoning)
}

func defaultConfidenceProbeBuilder(claim string) string {
	return fmt.Sprintf(defaultConfidenceProbe, claim)
}
