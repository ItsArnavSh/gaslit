package engine

import (
	"context"

	"github.com/ItsArnavSh/gaslit/internal/core"
)

// Stream runs Debate and streams each milestone over a channel as it happens.
// The caller must drain the channel; it is closed when the debate finishes.
type StreamEvent = core.StreamEvent

// Stream runs a full debate and emits events as they occur.
func (g *Gaslit) Stream(ctx context.Context, question string, options []string, opts ...core.CallOption) (<-chan core.StreamEvent, error) {
	ch := make(chan core.StreamEvent, 64)
	send := func(ev core.StreamEvent) {
		select {
		case ch <- ev:
		case <-ctx.Done():
		}
	}

	fw := core.Hooks{
		OnPhaseStart: func(_ context.Context, phase core.Phase) {
			send(core.StreamEvent{Type: core.EventPhaseStart, Phase: phase})
		},
		OnPhaseEnd: func(_ context.Context, phase core.Phase) {
			send(core.StreamEvent{Type: core.EventPhaseEnd, Phase: phase})
		},
		OnBaseline: func(_ context.Context, agent string, ans *core.Answer) {
			send(core.StreamEvent{Type: core.EventBaseline, Agent: agent, Answer: ans})
		},
		OnCrossExam: func(_ context.Context, target, prompt string) {
			send(core.StreamEvent{Type: core.EventCrossExam, Agent: target, Prompt: prompt})
		},
		OnEval: func(_ context.Context, agent string, ans *core.Answer) {
			send(core.StreamEvent{Type: core.EventEval, Agent: agent, Answer: ans})
		},
		OnDrift: func(_ context.Context, verdict *core.DriftVerdict) {
			send(core.StreamEvent{Type: core.EventDrift, Verdict: verdict})
		},
		OnDiagnostic: func(_ context.Context, diag *core.DiagnosticResult) {
			send(core.StreamEvent{Type: core.EventDiagnostic, Agent: diag.AgentName, Diagnostic: diag})
		},
	}
	opts = append(opts, core.WithHooksCall(fw))

	go func() {
		defer close(ch)
		_, err := g.Debate(ctx, question, options, opts...)
		if err != nil {
			send(core.StreamEvent{Type: core.EventError, Error: err})
			return
		}
		send(core.StreamEvent{Type: core.EventDone})
	}()

	return ch, nil
}
