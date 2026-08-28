package gaslit

import "context"

// Stream runs Debate and streams each milestone over a channel as it happens.
// The caller must drain the channel; it is closed when the debate finishes.
type StreamEvent struct {
	Type       EventType         `json:"type"`
	Phase      Phase             `json:"phase,omitempty"`
	Agent      string            `json:"agent,omitempty"`
	Prompt     string            `json:"prompt,omitempty"`
	Answer     *Answer           `json:"answer,omitempty"`
	Verdict    *DriftVerdict     `json:"verdict,omitempty"`
	Diagnostic *DiagnosticResult `json:"diagnostic,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	Error      error             `json:"-"`
}

// Stream runs a full debate and emits events as they occur.
func (g *Gaslit) Stream(ctx context.Context, question string, options []string, opts ...CallOption) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 64)
	send := func(ev StreamEvent) {
		select {
		case ch <- ev:
		case <-ctx.Done():
		}
	}

	fw := Hooks{
		OnPhaseStart: func(_ context.Context, phase Phase) {
			send(StreamEvent{Type: EventPhaseStart, Phase: phase})
		},
		OnPhaseEnd: func(_ context.Context, phase Phase) {
			send(StreamEvent{Type: EventPhaseEnd, Phase: phase})
		},
		OnBaseline: func(_ context.Context, agent string, ans *Answer) {
			send(StreamEvent{Type: EventBaseline, Agent: agent, Answer: ans})
		},
		OnCrossExam: func(_ context.Context, target, prompt string) {
			send(StreamEvent{Type: EventCrossExam, Agent: target, Prompt: prompt})
		},
		OnEval: func(_ context.Context, agent string, ans *Answer) {
			send(StreamEvent{Type: EventEval, Agent: agent, Answer: ans})
		},
		OnDrift: func(_ context.Context, verdict *DriftVerdict) {
			send(StreamEvent{Type: EventDrift, Verdict: verdict})
		},
		OnDiagnostic: func(_ context.Context, diag *DiagnosticResult) {
			send(StreamEvent{Type: EventDiagnostic, Agent: diag.AgentName, Diagnostic: diag})
		},
	}
	opts = append(opts, WithHooksCall(fw))

	go func() {
		defer close(ch)
		_, err := g.Debate(ctx, question, options, opts...)
		if err != nil {
			send(StreamEvent{Type: EventError, Error: err})
			return
		}
		send(StreamEvent{Type: EventDone})
	}()

	return ch, nil
}
