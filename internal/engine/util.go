package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func sanitizeHistoryMessage(msg *schema.Message) *schema.Message {
	if msg == nil {
		return nil
	}
	return &schema.Message{
		Role:    msg.Role,
		Content: msg.Content,
	}
}

const retryHint = "Your previous response could not be parsed as valid JSON. Respond again with ONLY the required JSON object matching the instructions you were given."

// queryAgent sends a prompt to a named agent, parses the answer through the
// schema, and re-prompts (up to MaxRetries times) when the output is malformed.
func (g *Gaslit) queryAgent(ctx context.Context, name, prompt string, resetHistory bool, as core.AnswerSchema) (*core.Answer, error) {
	st := g.states[name]

	st.mu.Lock()
	defer st.mu.Unlock()
	if resetHistory {
		st.history = []*schema.Message{schema.UserMessage(prompt)}
	} else {
		st.history = append(st.history, schema.UserMessage(prompt))
	}

	var lastErr error
	for attempt := 0; attempt <= g.cfg.MaxRetries; attempt++ {
		resp, err := g.runAgent(ctx, st, st.history)
		if err != nil {
			return nil, fmt.Errorf("agent %s execution error: %w", name, err)
		}
		st.history = append(st.history, sanitizeHistoryMessage(resp))

		parsed, perr := as.Parse(resp.Content)
		if perr == nil {
			return parsed, nil
		}
		lastErr = perr
		if attempt < g.cfg.MaxRetries {
			st.history = append(st.history, schema.UserMessage(retryHint))
		}
	}

	return nil, fmt.Errorf("agent %s failed to produce parseable output after %d attempts: %w", name, g.cfg.MaxRetries+1, lastErr)
}

// sendToAgent sends a free-form prompt to an agent and returns its raw reply.
// Used by cross-examination and grilling probes (no JSON parsing).
func (g *Gaslit) sendToAgent(ctx context.Context, st *agentState, prompt string) (*schema.Message, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.history = append(st.history, schema.UserMessage(prompt))
	resp, err := g.runAgent(ctx, st, st.history)
	if err != nil {
		return nil, err
	}
	st.history = append(st.history, sanitizeHistoryMessage(resp))
	return resp, nil
}

// runAgent executes a single model invocation with pacing, budget accounting,
// and an optional per-request timeout.
func (g *Gaslit) runAgent(ctx context.Context, st *agentState, msgs []*schema.Message) (*schema.Message, error) {
	if err := g.checkBudget(ctx); err != nil {
		return nil, err
	}
	if err := g.pace(ctx); err != nil {
		return nil, err
	}

	rctx, cancel := ctx, func() {}
	if g.cfg.RequestTimeout > 0 {
		rctx, cancel = context.WithTimeout(ctx, g.cfg.RequestTimeout)
	}
	defer cancel()

	iter := st.agent.Run(rctx, &adk.TypedAgentInput[*schema.Message]{Messages: msgs})
	resp, err := consumeAgentOutput(iter)
	if err != nil {
		return nil, err
	}
	g.accountCall(st.agent.Name(rctx), resp.Content)
	return resp, nil
}

// forEachAgent runs fn for every agent in names, honoring the engine's
// Concurrency setting. fn is called with the agent's name and is guaranteed to
// never run concurrently for the same agent.
func (g *Gaslit) forEachAgent(ctx context.Context, names []string, fn func(ctx context.Context, name string) error) error {
	if g.cfg.Concurrency <= 1 {
		for _, name := range names {
			if err := fn(ctx, name); err != nil {
				return err
			}
		}
		return nil
	}

	sem := make(chan struct{}, g.cfg.Concurrency)
	errCh := make(chan error, len(names))
	var wg sync.WaitGroup

	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := fn(ctx, name); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// agentInstruction returns the effective system instruction for an agent: the
// per-agent override when set, otherwise an empty string (the agent's own
// default applies).
func agentInstruction(st *agentState) string {
	if st.cfg.Instruction != "" {
		return st.cfg.Instruction + "\n\n"
	}
	return ""
}
