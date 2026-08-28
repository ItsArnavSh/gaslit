package gaslit

import "context"

// Hooks are observability callbacks fired at pipeline milestones. Nil fields
// are skipped. All hooks receive the debate's context.
type Hooks struct {
	OnPhaseStart func(ctx context.Context, phase Phase)
	OnPhaseEnd   func(ctx context.Context, phase Phase)
	OnBaseline   func(ctx context.Context, agent string, ans *Answer)
	OnCrossExam  func(ctx context.Context, target, prompt string)
	OnEval       func(ctx context.Context, agent string, ans *Answer)
	OnDrift      func(ctx context.Context, verdict *DriftVerdict)
	OnDiagnostic func(ctx context.Context, diag *DiagnosticResult)
}

// mergeHooks combines a base set with overrides; non-nil override fields take
// precedence.
func mergeHooks(base, override Hooks) Hooks {
	out := base
	if override.OnPhaseStart != nil {
		out.OnPhaseStart = override.OnPhaseStart
	}
	if override.OnPhaseEnd != nil {
		out.OnPhaseEnd = override.OnPhaseEnd
	}
	if override.OnBaseline != nil {
		out.OnBaseline = override.OnBaseline
	}
	if override.OnCrossExam != nil {
		out.OnCrossExam = override.OnCrossExam
	}
	if override.OnEval != nil {
		out.OnEval = override.OnEval
	}
	if override.OnDrift != nil {
		out.OnDrift = override.OnDrift
	}
	if override.OnDiagnostic != nil {
		out.OnDiagnostic = override.OnDiagnostic
	}
	return out
}
