package gaslit

import (
	"sync"
	"time"
)

// instrumentation tracks phase durations and per-agent call counts. It is
// always enabled and snapshotted via Stats.
type instrumentation struct {
	mu            sync.Mutex
	phaseStarted  map[Phase]time.Time
	phaseDuration map[Phase][]time.Duration
	agentCalls    map[string]int64
}

func newInstrumentation() *instrumentation {
	return &instrumentation{
		phaseStarted:  make(map[Phase]time.Time),
		phaseDuration: make(map[Phase][]time.Duration),
		agentCalls:    make(map[string]int64),
	}
}

func (i *instrumentation) phaseStart(phase Phase) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.phaseStarted[phase]; !ok {
		i.phaseStarted[phase] = time.Now()
	}
}

func (i *instrumentation) phaseEnd(phase Phase) {
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

// InstrumentationStats is a point-in-time snapshot of engine usage.
type InstrumentationStats struct {
	TotalCalls    int64
	Tokens        int64
	AgentCalls    map[string]int64
	PhaseDuration map[string][]time.Duration
}

// Stats returns a snapshot of the engine's instrumentation counters.
func (g *Gaslit) Stats() InstrumentationStats {
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

	return InstrumentationStats{
		TotalCalls:    g.calls.Load(),
		Tokens:        g.tokens.Load(),
		AgentCalls:    agentCalls,
		PhaseDuration: phaseDuration,
	}
}
