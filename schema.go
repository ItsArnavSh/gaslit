package gaslit

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

// schemaFor returns the schema to use for a given question. Questions with no
// options always use the open-ended schema.
func (g *Gaslit) schemaFor(options []string) AnswerSchema {
	if len(options) == 0 {
		return &OpenEndedSchema{}
	}
	return g.cfg.AnswerSchema
}

// extractJSONObject finds the outermost balanced JSON object in raw output.
func extractJSONObject(raw string) (string, error) {
	end := strings.LastIndex(raw, "}")
	if end == -1 {
		return "", fmt.Errorf("no closing brace found in response")
	}

	depth := 0
	start := -1
	for i := end; i >= 0; i-- {
		switch raw[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				start = i
			}
		}
		if start != -1 {
			break
		}
	}
	if start == -1 {
		return "", fmt.Errorf("no matching opening brace found for JSON object")
	}
	return raw[start : end+1], nil
}

// unmarshalAnswer extracts and decodes an Answer from raw output, optionally
// requiring per-option confidences.
func unmarshalAnswer(raw string, requireConfidences bool) (*Answer, error) {
	jsonStr, err := extractJSONObject(raw)
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
