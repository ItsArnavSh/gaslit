package engine

import (
	"fmt"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

// resolveCall merges per-call options over the engine's configuration.
func (g *Gaslit) resolveCall(opts ...core.CallOption) core.CallOptions {
	co := core.CallOptions{
		MaxRounds:         g.cfg.MaxRounds,
		OptionLabels:      g.cfg.OptionLabels,
		Thresholds:        g.cfg.Thresholds,
		GrillStrategy:     g.cfg.GrillStrategy,
		GrillCount:        g.cfg.GrillCount,
		Protocol:          g.cfg.Protocol,
		Verbose:           g.cfg.Verbose,
		Budget:            g.cfg.Budget,
		ConsistencyScorer: g.cfg.ConsistencyScorer,
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
func (g *Gaslit) hooks(co core.CallOptions) core.Hooks {
	if co.Hooks != nil {
		return core.MergeHooks(g.cfg.Hooks, *co.Hooks)
	}
	return g.cfg.Hooks
}

// logf logs a verbose progress line through the configured logger.
func (g *Gaslit) logf(co core.CallOptions, format string, args ...any) {
	if !co.Verbose {
		return
	}
	g.logger.Info(fmt.Sprintf(format, args...))
}
