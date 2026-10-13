// Package qa generates multiple-choice questionnaires from parsed book sources
// chapter by chapter, using a caller-supplied model, and persists them as a
// JSON question bank.
package qa

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Question is a single generated multiple-choice question with its ground truth.
type Question struct {
	ID           string   `json:"id"`
	Source       string   `json:"source"`
	Chapter      int      `json:"chapter"`
	ChapterTitle string   `json:"chapter_title"`
	Question     string   `json:"question"`
	Options      []string `json:"options"`
	Answer       int      `json:"answer"` // 0-based index of the correct option
	Explanation  string   `json:"explanation,omitempty"`
	Difficulty   string   `json:"difficulty,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// QuestionBank is a persisted set of generated questions together with the
// configuration and sources used to produce them.
type QuestionBank struct {
	CreatedAt time.Time  `json:"created_at"`
	Sources   []string   `json:"sources"`
	Config    Config     `json:"config"`
	Questions []Question `json:"questions"`
}

// Config controls questionnaire generation.
type Config struct {
	// Sources limits generation to these source names. Empty means all.
	Sources []string `json:"sources,omitempty"`
	// PerChapter is the number of questions to request from each chapter.
	PerChapter int `json:"per_chapter,omitempty"`
	// Total, when PerChapter is zero, is distributed across the selected
	// chapters.
	Total int `json:"total,omitempty"`
	// MaxChapters caps how many chapters are sampled (spread evenly across the
	// whole book). Zero means all.
	MaxChapters int `json:"max_chapters,omitempty"`
	// Options is the number of answer choices per question. Default 4.
	Options int `json:"options,omitempty"`
	// Difficulty is the requested difficulty: easy, medium, hard, or mixed.
	// Default "hard".
	Difficulty string `json:"difficulty,omitempty"`
	// Style customizes the flavor of questions. Defaults to deep, subjective,
	// reasoning-heavy questions that avoid trivia.
	Style string `json:"style,omitempty"`
	// IncludeExplanation asks the model for a rationale for the correct answer.
	IncludeExplanation bool `json:"include_explanation,omitempty"`
	// MaxChapterChars bounds how much chapter text is sent per generation call;
	// longer chapters are summarized into key points first. Default 24000.
	MaxChapterChars int `json:"max_chapter_chars,omitempty"`
	// Language is the language of the generated questions. Default "English".
	Language string `json:"language,omitempty"`
	// Seed is an optional value included in prompts to nudge variety.
	Seed int64 `json:"seed,omitempty"`
}

const defaultStyle = "deep, subjective, interpretive questions that require reasoning over the whole passage; avoid trivia, dates, and simple factual recall"

// Resolve fills zero fields with their defaults.
func (c Config) Resolve() Config {
	if c.Options <= 0 {
		c.Options = 4
	}
	if c.Difficulty == "" {
		c.Difficulty = "hard"
	}
	if c.Style == "" {
		c.Style = defaultStyle
	}
	if c.MaxChapterChars <= 0 {
		c.MaxChapterChars = 24000
	}
	if c.Language == "" {
		c.Language = "English"
	}
	return c
}

// SaveBank writes a question bank to path as indented JSON.
func SaveBank(path string, bank *QuestionBank) error {
	if bank == nil {
		return fmt.Errorf("qa: nil question bank")
	}
	data, err := json.MarshalIndent(bank, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return nil
}

// LoadBank reads a question bank written by SaveBank.
func LoadBank(path string) (*QuestionBank, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bank QuestionBank
	if err := json.Unmarshal(data, &bank); err != nil {
		return nil, fmt.Errorf("qa: decode question bank: %w", err)
	}
	return &bank, nil
}

// Labels returns the option labels (A, B, C, ...) for a question.
func (q Question) Labels() []string { return Labels(len(q.Options)) }

// AnswerLabel returns the label of the correct option.
func (q Question) AnswerLabel() string {
	if q.Answer < 0 || q.Answer >= len(q.Options) {
		return ""
	}
	return string(rune('A' + q.Answer))
}

// Labels returns A, B, C, ... for n options.
func Labels(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('A' + i))
	}
	return out
}

// normalizeKey lowercases and strips punctuation for near-duplicate detection.
func normalizeKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
