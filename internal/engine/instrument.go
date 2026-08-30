package engine

import (
	"sync"
	"time"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

// instrumentation tracks phase durations and per-agent call counts. It is
// always enabled and snapshotted via Stats.
type instrumentation struct {
	mu            sync.Mutex
	phaseStarted  map[core.Phase]time.Time
	phaseDuration map[core.Phase][]time.Duration
	agentCalls    map[string]int64
}

func newInstrumentation() *instrumentation {
	return &instrumentation{
		phaseStarted:  make(map[core.Phase]time.Time),
		phaseDuration: make(map[core.Phase][]time.Duration),
		agentCalls:    make(map[string]int64),
	}
}

func (i *instrumentation) phaseStart(phase core.Phase) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.phaseStarted[phase]; !ok {
		i.phaseStarted[phase] = time.Now()
	}
}

func (i *instrumentation) phaseEnd(phase core.Phase) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if start, ok := i.phaseStarted[phase]; ok {
		i.phaseDuration[phase] = append(i.phaseDuration[phase], time.Since(start))
		delete(i.phaseStarted, phase)
	}
}

func (i *instrumentation) recordAgentCall(agent string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.agentCalls[agent]++
}

// Stats returns a snapshot of the engine's instrumentation counters.
func (g *Gaslit) Stats() core.InstrumentationStats {
	g.instr.mu.Lock()
	defer g.instr.mu.Unlock()

	agentCalls := make(map[string]int64, len(g.instr.agentCalls))
	for k, v := range g.instr.agentCalls {
		agentCalls[k] = v
	}
	phaseDuration := make(map[string][]time.Duration, len(g.instr.phaseDuration))
	for k, v := range g.instr.phaseDuration {
		durs := make([]time.Duration, len(v))
		copy(durs, v)
		phaseDuration[k.String()] = durs
	}

	return core.InstrumentationStats{
		TotalCalls:    g.calls.Load(),
		Tokens:        g.tokens.Load(),
		AgentCalls:    agentCalls,
		PhaseDuration: phaseDuration,
	}
}
