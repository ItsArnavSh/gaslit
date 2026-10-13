package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ItsArnavSh/gaslit/internal/corpus"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Generator produces question banks from parsed sources using a model.
type Generator struct {
	// Model authors the questions.
	Model model.BaseModel[*schema.Message]
	// Logger receives progress output. Optional.
	Logger *slog.Logger
	// MaxRetries is the number of corrective re-prompts on unparseable output.
	// Default 2.
	MaxRetries int
	// System overrides the built-in system prompt.
	System string
}

// NewGenerator returns a Generator for the given model.
func NewGenerator(m model.BaseModel[*schema.Message], logger *slog.Logger) *Generator {
	return &Generator{Model: m, Logger: logger}
}

func (g *Generator) maxRetries() int {
	if g.MaxRetries <= 0 {
		return 2
	}
	return g.MaxRetries
}

func (g *Generator) logf(format string, args ...any) {
	if g.Logger != nil {
		g.Logger.Info(fmt.Sprintf(format, args...))
	}
}

// rawQuestion mirrors the JSON schema requested from the model.
type rawQuestion struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Answer      int      `json:"answer"`
	Explanation string   `json:"explanation"`
	Difficulty  string   `json:"difficulty"`
	Tags        []string `json:"tags"`
}

// Generate produces a question bank from the given sources. When cfg.Sources is
// set, only those sources are used.
func (g *Generator) Generate(ctx context.Context, sources []*corpus.Source, cfg Config) (*QuestionBank, error) {
	if g.Model == nil {
		return nil, fmt.Errorf("qa: Generator.Model is required")
	}
	cfg = cfg.Resolve()

	selected := filterSources(sources, cfg.Sources)
	if len(selected) == 0 {
		return nil, fmt.Errorf("qa: no sources matched %v", cfg.Sources)
	}

	refs := collectChapters(selected)
	if len(refs) == 0 {
		return nil, fmt.Errorf("qa: no chapters with usable text were found")
	}
	refs = selectChapters(refs, cfg.MaxChapters)
	alloc := allocate(len(refs), cfg)

	bank := &QuestionBank{CreatedAt: time.Now().UTC(), Config: cfg}
	for _, s := range selected {
		bank.Sources = append(bank.Sources, s.Name)
	}

	seen := map[string]bool{}
	serial := 0
	for i, ref := range refs {
		n := alloc[i]
		if n <= 0 {
			continue
		}
		g.logf("generating %d question(s) from %s / %s", n, ref.source.Name, ref.chapter.Title)
		qs, err := g.generateChapter(ctx, cfg, ref.source, ref.chapter, n)
		if err != nil {
			g.logf("  ! chapter %s failed: %v", ref.chapter.Title, err)
			continue
		}
		for _, q := range qs {
			key := normalizeKey(q.Question)
			if seen[key] {
				continue
			}
			seen[key] = true
			serial++
			q.ID = fmt.Sprintf("q%03d", serial)
			bank.Questions = append(bank.Questions, q)
		}
	}

	if len(bank.Questions) == 0 {
		return nil, fmt.Errorf("qa: generation produced no questions")
	}
	return bank, nil
}

type chapterRef struct {
	source  *corpus.Source
	chapter corpus.Chapter
}

func filterSources(all []*corpus.Source, names []string) []*corpus.Source {
	if len(names) == 0 {
		return all
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []*corpus.Source
	for _, s := range all {
		if want[s.Name] {
			out = append(out, s)
		}
	}
	return out
}

const minChapterChars = 400

func collectChapters(sources []*corpus.Source) []chapterRef {
	var refs []chapterRef
	for _, s := range sources {
		for _, ch := range s.Chapters {
			if len(ch.Text) < minChapterChars {
				continue
			}
			refs = append(refs, chapterRef{source: s, chapter: ch})
		}
	}
	return refs
}

// selectChapters caps the number of chapters, sampling evenly across the book
// so questions span the whole source.
func selectChapters(refs []chapterRef, max int) []chapterRef {
	if max <= 0 || len(refs) <= max {
		return refs
	}
	out := make([]chapterRef, 0, max)
	for i := 0; i < max; i++ {
		idx := int(float64(i) * float64(len(refs)) / float64(max))
		out = append(out, refs[idx])
	}
	return out
}

// allocate decides how many questions to request per chapter.
func allocate(n int, cfg Config) []int {
	alloc := make([]int, n)
	switch {
	case cfg.PerChapter > 0:
		for i := range alloc {
			alloc[i] = cfg.PerChapter
		}
	case cfg.Total > 0 && n > 0:
		base := cfg.Total / n
		rem := cfg.Total % n
		for i := range alloc {
			alloc[i] = base
			if i < rem {
				alloc[i]++
			}
		}
	default:
		for i := range alloc {
			alloc[i] = 3
		}
	}
	return alloc
}

func (g *Generator) generateChapter(ctx context.Context, cfg Config, src *corpus.Source, ch corpus.Chapter, n int) ([]Question, error) {
	text := ch.Text
	if len(text) > cfg.MaxChapterChars {
		kp, err := g.complete(ctx, keyPointsPrompt(cfg, ch.Title, truncate(text, cfg.MaxChapterChars)))
		if err == nil && strings.TrimSpace(kp) != "" {
			text = truncate(text, cfg.MaxChapterChars/2) + "\n\nKEY POINTS:\n" + kp
		} else {
			text = truncate(text, cfg.MaxChapterChars)
		}
	}

	base := chapterPrompt(cfg, src.Title, ch.Title, n, text)
	prompt := base
	var lastErr error
	for attempt := 0; attempt <= g.maxRetries(); attempt++ {
		raw, err := g.complete(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		qs, err := parseQuestions(raw, cfg)
		if err == nil && len(qs) > 0 {
			for i := range qs {
				qs[i].Source = src.Name
				qs[i].Chapter = ch.Index
				qs[i].ChapterTitle = ch.Title
			}
			return qs, nil
		}
		lastErr = err
		prompt = base + "\n\n" + retryHint
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no parseable questions returned")
	}
	return nil, lastErr
}

// complete runs one model call with the question-generation system prompt.
func (g *Generator) complete(ctx context.Context, user string) (string, error) {
	system := g.System
	if system == "" {
		system = systemPrompt
	}
	resp, err := g.Model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(system),
		schema.UserMessage(user),
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func parseQuestions(raw string, cfg Config) ([]Question, error) {
	arr, err := extractJSONArray(raw)
	if err != nil {
		return nil, err
	}
	var rqs []rawQuestion
	if err := json.Unmarshal([]byte(arr), &rqs); err != nil {
		return nil, fmt.Errorf("qa: decode questions: %w", err)
	}
	var out []Question
	for _, rq := range rqs {
		q, ok := rq.toQuestion(cfg)
		if !ok {
			continue
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("qa: no valid questions in model output")
	}
	return out, nil
}

func (rq rawQuestion) toQuestion(cfg Config) (Question, bool) {
	q := Question{
		Question:    strings.TrimSpace(rq.Question),
		Explanation: strings.TrimSpace(rq.Explanation),
		Difficulty:  strings.TrimSpace(rq.Difficulty),
		Tags:        rq.Tags,
	}
	for _, o := range rq.Options {
		o = strings.TrimSpace(o)
		if o != "" {
			q.Options = append(q.Options, o)
		}
	}
	if q.Question == "" || len(q.Options) < 2 {
		return Question{}, false
	}
	if rq.Answer < 0 || rq.Answer >= len(q.Options) {
		return Question{}, false
	}
	q.Answer = rq.Answer
	if q.Difficulty == "" {
		q.Difficulty = cfg.Difficulty
	}
	if !cfg.IncludeExplanation {
		q.Explanation = ""
	}
	return q, true
}

// extractJSONArray returns the outermost balanced [...] in raw output.
func extractJSONArray(raw string) (string, error) {
	start := strings.IndexByte(raw, '[')
	if start < 0 {
		return "", fmt.Errorf("qa: no JSON array found in model output")
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return raw[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("qa: unterminated JSON array in model output")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
