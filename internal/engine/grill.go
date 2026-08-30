package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/scoring"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// CCI runs a standalone Confidence/Consistency probe against any agent on a
// given statement. The agent does not need to be part of the panel. Confidence
// defaults to the agent's self-reported confidence in the statement; override
// it with WithConfidenceCall.
func (g *Gaslit) CCI(ctx context.Context, agent adk.Agent, statement, question string, opts ...core.CallOption) (*core.DiagnosticResult, error) {
	co := g.resolveCall(opts...)
	hk := g.hooks(co)
	g.phaseStart(ctx, core.PhaseGrilling, hk)
	defer g.phaseEnd(ctx, core.PhaseGrilling, hk)

	ctx, cancel := g.withBudgetContext(ctx, co.Budget)
	defer cancel()

	st := &agentState{agent: agent, history: make([]*schema.Message, 0)}
	name := agent.Name(ctx)

	claim := statement
	if co.Claim != "" {
		claim = co.Claim
	}

	confScore := 0.0
	if co.Confidence != nil {
		confScore = *co.Confidence
	} else {
		var err error
		confScore, err = g.selfConfidence(ctx, st, claim)
		if err != nil {
			return nil, fmt.Errorf("confidence probe failed: %w", err)
		}
	}

	r2, err := g.inversionProbe(ctx, st, question, claim)
	if err != nil {
		return nil, err
	}

	consistency, err := g.score(ctx, co, question, statement, r2)
	if err != nil {
		return nil, fmt.Errorf("consistency evaluation failed: %w", err)
	}

	quadrant, recommendation := core.ClassifyWith(confScore, consistency, co.Thresholds)
	diag := &core.DiagnosticResult{
		AgentName:        name,
		ConfidenceScore:  confScore,
		ConsistencyScore: consistency,
		Quadrant:         quadrant,
		InitialReasonR1:  statement,
		CounterReasonR2:  r2,
		Recommendation:   recommendation,
	}
	if hk.OnDiagnostic != nil {
		hk.OnDiagnostic(ctx, diag)
	}
	g.record(core.EventDiagnostic, name, diag)
	return diag, nil
}

// Grill runs a grilling diagnostic against a registered panelist on a claim.
func (g *Gaslit) Grill(ctx context.Context, agentName, claim, question string, opts ...core.CallOption) (*core.DiagnosticResult, error) {
	co := g.resolveCall(opts...)
	hk := g.hooks(co)

	ctx, cancel := g.withBudgetContext(ctx, co.Budget)
	defer cancel()

	st, ok := g.states[agentName]
	if !ok {
		return nil, fmt.Errorf("agent %q is not registered", agentName)
	}

	confScore := 0.0
	if co.Confidence != nil {
		confScore = *co.Confidence
	} else {
		var err error
		confScore, err = g.selfConfidence(ctx, st, claim)
		if err != nil {
			return nil, fmt.Errorf("confidence probe failed: %w", err)
		}
	}

	r2, err := g.inversionProbe(ctx, st, question, claim)
	if err != nil {
		return nil, err
	}

	consistency, err := g.score(ctx, co, question, claim, r2)
	if err != nil {
		return nil, fmt.Errorf("consistency evaluation failed: %w", err)
	}

	quadrant, recommendation := core.ClassifyWith(confScore, consistency, co.Thresholds)
	diag := &core.DiagnosticResult{
		AgentName:        agentName,
		ConfidenceScore:  confScore,
		ConsistencyScore: consistency,
		Quadrant:         quadrant,
		InitialReasonR1:  claim,
		CounterReasonR2:  r2,
		Recommendation:   recommendation,
	}
	if hk.OnDiagnostic != nil {
		hk.OnDiagnostic(ctx, diag)
	}
	g.record(core.EventDiagnostic, agentName, diag)
	return diag, nil
}

// selfConfidence asks an agent how confident it is in a claim, on [0, 1].
func (g *Gaslit) selfConfidence(ctx context.Context, st *agentState, claim string) (float64, error) {
	probe := g.cfg.PromptTemplates.ConfidenceProbe(g.redact(claim))
	resp, err := g.sendToAgent(ctx, st, probe)
	if err != nil {
		return 0, err
	}
	return parseConfidence(resp.Content)
}

// inversionProbe forces an agent to defend the position opposite its claim and
// returns the counter-argument (R2).
func (g *Gaslit) inversionProbe(ctx context.Context, st *agentState, question, claim string) (string, error) {
	probe := g.cfg.PromptTemplates.GrillProbe(g.redact(question), g.redact(claim))
	resp, err := g.sendToAgent(ctx, st, probe)
	if err != nil {
		return "", fmt.Errorf("grilling probe failed: %w", err)
	}
	return resp.Content, nil
}

// score evaluates the consistency between two arguments from the same agent.
// The effective scorer comes from the per-call options (falling back to the
// engine config). Deterministic scorers (e.g. LexicalScorer) skip budget
// accounting, pacing, and the model-call counter since they make no model
// calls.
func (g *Gaslit) score(ctx context.Context, co core.CallOptions, question, r1, r2 string) (float64, error) {
	scorer := co.ConsistencyScorer
	if scorer == nil {
		scorer = g.cfg.ConsistencyScorer
	}
	_, isLexical := scorer.(*scoring.LexicalScorer)
	if !isLexical {
		if err := g.checkBudget(ctx); err != nil {
			return 0, err
		}
		if err := g.pace(ctx); err != nil {
			return 0, err
		}
	}
	s, err := scorer.Score(ctx, g.redact(question), g.redact(r1), g.redact(r2))
	if !isLexical {
		g.accountCall("consistency_scorer", r1+r2)
	}
	return s, err
}

// grillPhase runs grilling diagnostics for the agents selected by the active
// GrillStrategy and returns them keyed by agent name.
func (g *Gaslit) grillPhase(ctx context.Context, question string, res *core.PromptResult, verdict core.DriftVerdict, labels []string, co core.CallOptions) map[string]*core.DiagnosticResult {
	hk := g.hooks(co)
	g.phaseStart(ctx, core.PhaseGrilling, hk)
	defer g.phaseEnd(ctx, core.PhaseGrilling, hk)

	diagnostics := make(map[string]*core.DiagnosticResult)
	if !verdict.HasDrift {
		return diagnostics
	}

	for _, name := range grillTargets(verdict, co) {
		b := res.Before[name]
		if b == nil {
			continue
		}
		conf := confidenceSlice(b, labels)
		diag, err := g.grillWithBaseline(ctx, name, question, b.Answer, b.Reasoning, conf, co)
		if err != nil {
			g.logf(co, ">>> [PROBING] %s skipped: %v", name, err)
			continue
		}
		diagnostics[name] = diag
	}
	return diagnostics
}

func (g *Gaslit) grillWithBaseline(ctx context.Context, name, question, claim, r1 string, conf []float64, co core.CallOptions) (*core.DiagnosticResult, error) {
	hk := g.hooks(co)
	st, ok := g.states[name]
	if !ok {
		return nil, fmt.Errorf("agent %q is not registered", name)
	}
	g.logf(co, ">>> [PROBING] %s ...", name)

	confScore := mean(conf)
	r2, err := g.inversionProbe(ctx, st, question, claim)
	if err != nil {
		return nil, err
	}

	consistency, err := g.score(ctx, co, question, r1, r2)
	if err != nil {
		return nil, fmt.Errorf("consistency evaluation failed: %w", err)
	}

	quadrant, recommendation := core.ClassifyWith(confScore, consistency, co.Thresholds)
	diag := &core.DiagnosticResult{
		AgentName:        name,
		ConfidenceScore:  confScore,
		ConsistencyScore: consistency,
		Quadrant:         quadrant,
		InitialReasonR1:  r1,
		CounterReasonR2:  r2,
		Recommendation:   recommendation,
	}
	if hk.OnDiagnostic != nil {
		hk.OnDiagnostic(ctx, diag)
	}
	g.record(core.EventDiagnostic, name, diag)
	return diag, nil
}

// grillTargets selects which agents to grill for the active strategy.
func grillTargets(verdict core.DriftVerdict, co core.CallOptions) []string {
	switch co.GrillStrategy {
	case core.GrillNone:
		return nil
	case core.GrillAllDrifted:
		return verdict.DriftedAgents
	case core.GrillTopN:
		return topDrifted(verdict, co.GrillCount)
	case core.GrillInfluencer:
		if verdict.PrimaryDriver != "None" && verdict.PrimaryDriver != "Supervisor (Cross-Examination)" {
			return []string{verdict.PrimaryDriver}
		}
		return topDrifted(verdict, 1)
	default: // GrillSingleSuspect
		if verdict.PrimaryDriver != "None" && verdict.PrimaryDriver != "Supervisor (Cross-Examination)" {
			return []string{verdict.PrimaryDriver}
		}
		return topDrifted(verdict, 1)
	}
}

func topDrifted(verdict core.DriftVerdict, n int) []string {
	type kv struct {
		name  string
		shift float64
	}
	items := make([]kv, 0, len(verdict.DriftedAgents))
	for _, name := range verdict.DriftedAgents {
		items = append(items, kv{name: name, shift: verdict.AgentShifts[name]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].shift > items[j].shift })
	if n > len(items) {
		n = len(items)
	}
	out := make([]string, 0, n)
	for _, it := range items[:n] {
		out = append(out, it.name)
	}
	return out
}

func confidenceSlice(a *core.Answer, labels []string) []float64 {
	out := make([]float64, 0, len(labels))
	for _, l := range labels {
		out = append(out, a.Confidences[l])
	}
	return out
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func parseConfidence(raw string) (float64, error) {
	jsonStr, err := core.ExtractJSONObject(raw)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return 0, err
	}
	if parsed.Confidence < 0 || parsed.Confidence > 1 {
		return 0, fmt.Errorf("confidence out of range: %v", parsed.Confidence)
	}
	return parsed.Confidence, nil
}
