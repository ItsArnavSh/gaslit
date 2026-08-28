package gaslit

import "fmt"

// CallOption customizes a single engine call (Debate, Baseline, Grill, CCI, ...)
// without mutating the engine's construction-time configuration.
type CallOption func(*CallOptions)

// CallOptions is the resolved per-call configuration.
type CallOptions struct {
	Verbose       bool
	MaxRounds     int
	OptionLabels  []string
	Thresholds    Thresholds
	GrillStrategy GrillStrategy
	GrillCount    int
	Protocol      Protocol
	Metadata      map[string]any
	Confidence    *float64
	Claim         string
	Hooks         *Hooks
	Budget        CostBudget
}

func WithVerboseCall(v bool) CallOption          { return func(o *CallOptions) { o.Verbose = v } }
func WithMaxRoundsCall(n int) CallOption         { return func(o *CallOptions) { o.MaxRounds = n } }
func WithOptionLabelsCall(l []string) CallOption { return func(o *CallOptions) { o.OptionLabels = l } }
func WithThresholdsCall(t Thresholds) CallOption { return func(o *CallOptions) { o.Thresholds = t } }
func WithGrillStrategyCall(s GrillStrategy) CallOption {
	return func(o *CallOptions) { o.GrillStrategy = s }
}
func WithGrillCountCall(n int) CallOption    { return func(o *CallOptions) { o.GrillCount = n } }
func WithProtocolCall(p Protocol) CallOption { return func(o *CallOptions) { o.Protocol = p } }
func WithMetadataCall(m map[string]any) CallOption {
	return func(o *CallOptions) { o.Metadata = m }
}
func WithConfidenceCall(c float64) CallOption { return func(o *CallOptions) { o.Confidence = &c } }
func WithClaimCall(s string) CallOption       { return func(o *CallOptions) { o.Claim = s } }
func WithHooksCall(h Hooks) CallOption        { return func(o *CallOptions) { o.Hooks = &h } }
func WithBudgetCall(b CostBudget) CallOption  { return func(o *CallOptions) { o.Budget = b } }

// resolveCall merges per-call options over the engine's configuration.
func (g *Gaslit) resolveCall(opts ...CallOption) CallOptions {
	co := CallOptions{
		MaxRounds:     g.cfg.MaxRounds,
		OptionLabels:  g.cfg.OptionLabels,
		Thresholds:    g.cfg.Thresholds,
		GrillStrategy: g.cfg.GrillStrategy,
		GrillCount:    g.cfg.GrillCount,
		Protocol:      g.cfg.Protocol,
		Verbose:       g.cfg.Verbose,
		Budget:        g.cfg.Budget,
	}
	for _, opt := range opts {
		opt(&co)
	}
	if co.MaxRounds <= 0 {
		co.MaxRounds = g.cfg.MaxRounds
	}
	if co.GrillCount <= 0 {
		co.GrillCount = g.cfg.GrillCount
	}
	return co
}

// hooks returns the effective hooks for a call.
func (g *Gaslit) hooks(co CallOptions) Hooks {
	if co.Hooks != nil {
		return mergeHooks(g.cfg.Hooks, *co.Hooks)
	}
	return g.cfg.Hooks
}

// logf logs a verbose progress line through the configured logger.
func (g *Gaslit) logf(co CallOptions, format string, args ...any) {
	if !co.Verbose {
		return
	}
	g.logger.Info(fmt.Sprintf(format, args...))
}
