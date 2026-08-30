package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AnswerSchema defines the JSON contract agents must follow when answering, and
// how raw model output is parsed into an Answer.
type AnswerSchema interface {
	// Instruction is the output-contract text appended to prompts.
	Instruction() string
	// Parse extracts an Answer from raw model output. It must tolerate leading
	// prose, markdown fences, and trailing chatter.
	Parse(raw string) (*Answer, error)
}

// DefaultJSONInstruction is the output contract agents must follow for
// multiple-choice answers. Custom AnswerSchema implementations may provide
// their own instructions instead.
const DefaultJSONInstruction = `Respond with ONLY a valid JSON object (no markdown, no backticks, no extra prose):
{"answer": "<chosen option label, e.g., A>", "confidences": {"<label>": <float 0.0-1.0>, ...}, "reasoning": "<one sentence reasoning>", "challenge": {"agent_name": "<exact panelist name>", "prompt": "<the specific claim or weakness you want them confronted with>"}}
Every option label MUST be present in "confidences".
The "challenge" field is OPTIONAL — include it only if you want to nominate another panelist for cross-examination. Omit it entirely if you have no challenge to raise.`

// OpenEndedInstruction is the output contract for free-form (option-less)
// questions.
const OpenEndedInstruction = `Respond with ONLY a valid JSON object (no markdown, no backticks, no extra prose):
{"answer": "<your complete answer to the question>", "reasoning": "<brief reasoning>"}
The "answer" field must contain the full answer text.`

// MCQSchema is the default schema for multiple-choice questions: it requires an
// option label and a confidence for every option.
type MCQSchema struct{}

func (s *MCQSchema) Instruction() string { return DefaultJSONInstruction }

func (s *MCQSchema) Parse(raw string) (*Answer, error) {
	return unmarshalAnswer(raw, true)
}

// OpenEndedSchema is the schema for free-form questions: agents return their
// answer text with no option labels or confidences.
type OpenEndedSchema struct{}

func (s *OpenEndedSchema) Instruction() string { return OpenEndedInstruction }

func (s *OpenEndedSchema) Parse(raw string) (*Answer, error) {
	return unmarshalAnswer(raw, false)
}

// unmarshalAnswer extracts and decodes an Answer from raw output, optionally
// requiring per-option confidences.
func unmarshalAnswer(raw string, requireConfidences bool) (*Answer, error) {
	jsonStr, err := ExtractJSONObject(raw)
	if err != nil {
		return nil, err
	}

	var a Answer
	if err := json.Unmarshal([]byte(jsonStr), &a); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w (extracted: %s)", err, jsonStr)
	}
	if strings.TrimSpace(a.Answer) == "" {
		return nil, fmt.Errorf("answer field is empty in parsed response: %s", jsonStr)
	}
	if requireConfidences && len(a.Confidences) == 0 {
		return nil, fmt.Errorf("confidences map is empty in parsed response: %s", jsonStr)
	}
	return &a, nil
}
