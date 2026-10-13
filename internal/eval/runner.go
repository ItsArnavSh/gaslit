// Package eval runs agents against a generated question bank and aggregates the
// results into an analysis suitable for reporting.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/engine"
	"github.com/ItsArnavSh/gaslit/internal/qa"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Mode selects how agents are exercised against each question.
type Mode string

const (
	// ModeAnswerOnly asks each agent to answer every question directly.
	ModeAnswerOnly Mode = "answer_only"
	// ModeDebate runs a full gaslit debate per question and scores the winner.
	ModeDebate Mode = "debate"
	// ModeBoth runs answer-only first, then a debate per question.
	ModeBoth Mode = "both"
)

// Target is a named agent under test.
type Target struct {
	Name  string
	Agent adk.Agent
}

// RunConfig configures an evaluation run.
type RunConfig struct {
	// Mode selects the evaluation mode. Default ModeAnswerOnly.
	Mode Mode
	// Targets are the agents to test. Required.
	Targets []Target
	// SupervisorModel powers debate mode. Required for ModeDebate/ModeBoth.
	SupervisorModel model.BaseModel[*schema.Message]
	// EngineOptions are applied to the gaslit engine used in debate mode.
	EngineOptions []core.Option
	// Verbose enables engine progress logging in debate mode.
	Verbose bool
	// Logger receives progress output. Optional.
	Logger *slog.Logger
}

// AgentAnswer is a single agent's answer to a single question.
type AgentAnswer struct {
	Agent       string             `json:"agent"`
	Chosen      int                `json:"chosen"` // option index, -1 if unparsed
	Label       string             `json:"label"`
	Confidences map[string]float64 `json:"confidences,omitempty"`
	Reasoning   string             `json:"reasoning,omitempty"`
	Correct     bool               `json:"correct"`
	Err         string             `json:"error,omitempty"`
}

// QuestionResult bundles the per-question outcome.
type QuestionResult struct {
	Question      qa.Question               `json:"question"`
	Answers       map[string]*AgentAnswer   `json:"answers,omitempty"`
	Debate        *core.FullDebateSummary   `json:"debate,omitempty"`
	DebateWinner  int                       `json:"debate_winner"`
	DebateCorrect bool                      `json:"debate_correct"`
	DebateEngaged bool                      `json:"debate_engaged"`
}

// RunResult is the full outcome of an evaluation run.
type RunResult struct {
	Mode       Mode             `json:"mode"`
	Bank       *qa.QuestionBank `json:"bank"`
	Results    []QuestionResult `json:"results"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt time.Time        `json:"finished_at"`
}

// Run exercises the configured targets against the question bank.
func Run(ctx context.Context, bank *qa.QuestionBank, cfg RunConfig) (*RunResult, error) {
	if bank == nil || len(bank.Questions) == 0 {
		return nil, fmt.Errorf("eval: empty question bank")
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("eval: at least one target is required")
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeAnswerOnly
	}
	if (cfg.Mode == ModeDebate || cfg.Mode == ModeBoth) && cfg.SupervisorModel == nil {
		return nil, fmt.Errorf("eval: SupervisorModel is required for %s mode", cfg.Mode)
	}

	res := &RunResult{Mode: cfg.Mode, Bank: bank, StartedAt: time.Now().UTC()}

	var debateEngine *engine.Gaslit
	if cfg.Mode == ModeDebate || cfg.Mode == ModeBoth {
		agents := make([]adk.Agent, 0, len(cfg.Targets))
		for _, t := range cfg.Targets {
			agents = append(agents, t.Agent)
		}
		ecfg := core.PresetDefault()
		ecfg.SupervisorModel = cfg.SupervisorModel
		ecfg.Agents = agents
		ecfg.Verbose = cfg.Verbose
		if cfg.Logger != nil {
			ecfg.Logger = cfg.Logger
		}
		eng, err := engine.New(ctx, ecfg, cfg.EngineOptions...)
		if err != nil {
			return nil, fmt.Errorf("eval: build debate engine: %w", err)
		}
		debateEngine = eng
	}

	for i := range bank.Questions {
		q := bank.Questions[i]
		qr := QuestionResult{Question: q, Answers: map[string]*AgentAnswer{}, DebateWinner: -1}

		if cfg.Mode == ModeAnswerOnly || cfg.Mode == ModeBoth {
			for _, t := range cfg.Targets {
				qr.Answers[t.Name] = runAnswerOnly(ctx, t, q)
			}
		}
		if cfg.Mode == ModeDebate || cfg.Mode == ModeBoth {
			summary, err := debateEngine.Debate(ctx, q.Question, q.Options)
			if err != nil {
				logf(cfg.Logger, "debate failed for %s: %v", q.ID, err)
			} else {
				qr.Debate = summary
				qr.DebateEngaged = true
				qr.DebateWinner = labelIndex(summary.OptionLabels, summary.Winner)
				qr.DebateCorrect = qr.DebateWinner == q.Answer
			}
		}

		res.Results = append(res.Results, qr)
		logf(cfg.Logger, "[%d/%d] %s evaluated", i+1, len(bank.Questions), q.ID)
	}

	res.FinishedAt = time.Now().UTC()
	return res, nil
}

// runAnswerOnly asks one agent to answer one question and scores it.
func runAnswerOnly(ctx context.Context, t Target, q qa.Question) *AgentAnswer {
	labels := q.Labels()
	answer := &AgentAnswer{Agent: t.Name, Chosen: -1}

	prompt := answerPrompt(q, labels)
	resp, err := complete(ctx, t.Agent, []*schema.Message{schema.UserMessage(prompt)})
	if err != nil {
		answer.Err = err.Error()
		return answer
	}

	parsed, err := parseMCQ(resp, labels)
	if err != nil {
		answer.Err = err.Error()
		return answer
	}
	answer.Chosen = labelIndex(labels, parsed.Answer)
	answer.Label = strings.ToUpper(strings.TrimSpace(parsed.Answer))
	answer.Confidences = parsed.Confidences
	answer.Reasoning = parsed.Reasoning
	answer.Correct = answer.Chosen == q.Answer
	return answer
}

// answerPrompt builds the multiple-choice prompt used in answer-only mode.
func answerPrompt(q qa.Question, labels []string) string {
	var b strings.Builder
	b.WriteString("Answer the following multiple-choice question using your knowledge and any sources available to you.\n\n")
	b.WriteString("Question: " + q.Question + "\n\nOptions:\n")
	for i, opt := range q.Options {
		if i < len(labels) {
			fmt.Fprintf(&b, "%s) %s\n", labels[i], opt)
		}
	}
	b.WriteString("\n" + core.DefaultJSONInstruction)
	return b.String()
}

// parseMCQ parses an agent response into an Answer, tolerating responses that
// omit confidences.
func parseMCQ(raw string, labels []string) (*core.Answer, error) {
	if a, err := (&core.MCQSchema{}).Parse(raw); err == nil {
		return a, nil
	}
	jsonStr, err := core.ExtractJSONObject(raw)
	if err != nil {
		return nil, fmt.Errorf("no JSON object in response")
	}
	var loose struct {
		Answer      string             `json:"answer"`
		Confidences map[string]float64 `json:"confidences"`
		Reasoning   string             `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &loose); err != nil {
		return nil, err
	}
	if labelIndex(labels, loose.Answer) < 0 {
		return nil, fmt.Errorf("answer %q is not a valid option label", loose.Answer)
	}
	return &core.Answer{Answer: loose.Answer, Confidences: loose.Confidences, Reasoning: loose.Reasoning}, nil
}

// labelIndex returns the index of label within labels, or -1.
func labelIndex(labels []string, label string) int {
	label = strings.ToUpper(strings.TrimSpace(label))
	for i, l := range labels {
		if strings.ToUpper(l) == label {
			return i
		}
	}
	return -1
}

// complete drains an agent's event stream into its final text message.
func complete(ctx context.Context, ag adk.Agent, msgs []*schema.Message) (string, error) {
	iter := ag.Run(ctx, &adk.TypedAgentInput[*schema.Message]{Messages: msgs})
	if iter == nil {
		return "", fmt.Errorf("nil agent iterator")
	}
	var latest string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return "", event.Err
		}
		msg, _, err := adk.GetMessage(event)
		if err == nil && msg != nil && strings.TrimSpace(msg.Content) != "" {
			latest = msg.Content
		}
	}
	if strings.TrimSpace(latest) == "" {
		return "", fmt.Errorf("agent returned no text content")
	}
	return latest, nil
}

func logf(l *slog.Logger, format string, args ...any) {
	if l != nil {
		l.Info(fmt.Sprintf(format, args...))
	}
}
