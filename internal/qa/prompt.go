package qa

import (
	"fmt"
	"strings"
)

// systemPrompt frames the model as an expert exam author.
const systemPrompt = `You are an expert examiner who authors rigorous multiple-choice questions for a deep-reading comprehension benchmark. ` +
	`You care about testing genuine understanding of a text's arguments, themes, tensions, and reasoning — not surface recall. ` +
	`You always answer with strictly valid JSON and nothing else.`

// chapterPrompt builds the generation prompt for one chapter.
func chapterPrompt(cfg Config, bookTitle, chapterTitle string, n int, text string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Book: %s\n", bookTitle)
	fmt.Fprintf(&b, "Chapter: %s\n", chapterTitle)
	fmt.Fprintf(&b, "Language: %s\n", cfg.Language)
	fmt.Fprintf(&b, "Difficulty: %s\n", cfg.Difficulty)
	if cfg.Seed != 0 {
		fmt.Fprintf(&b, "Variation seed (use to diversify questions): %d\n", cfg.Seed)
	}
	fmt.Fprintf(&b, "\nWrite exactly %d multiple-choice questions about the chapter below.\n", n)
	fmt.Fprintf(&b, "Requirements:\n")
	fmt.Fprintf(&b, "- Style: %s.\n", cfg.Style)
	fmt.Fprintf(&b, "- Each question must have exactly %d options.\n", cfg.Options)
	fmt.Fprintf(&b, "- Exactly one option is unambiguously correct; the others must be plausible but wrong.\n")
	fmt.Fprintf(&b, "- Questions must be answerable from the chapter alone and must not refer to 'the passage' or 'the text'.\n")
	fmt.Fprintf(&b, "- Base the questions on the actual content of this chapter, not on general knowledge.\n")
	if cfg.IncludeExplanation {
		fmt.Fprintf(&b, "- Provide a one-sentence explanation for the correct answer.\n")
	}
	fmt.Fprintf(&b, "\nReturn ONLY a JSON array, with no markdown and no prose, in this exact schema:\n")
	fmt.Fprintf(&b, `[{"question":"...","options":["...","...","..."],"answer":0,"explanation":"...","difficulty":"%s","tags":["..."]}]`, cfg.Difficulty)
	fmt.Fprintf(&b, "\n\"answer\" is the 0-based index of the correct option.\n\nCHAPTER TEXT:\n%s\n", text)
	return b.String()
}

// keyPointsPrompt builds the first pass for chapters too long to send whole.
func keyPointsPrompt(cfg Config, chapterTitle, text string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Chapter: %s\n", chapterTitle)
	fmt.Fprintf(&b, "The following is an excerpt from a chapter. Extract the %d most important, distinctive claims, arguments, and themes as a numbered list. Preserve specific terminology, positions, and tensions in the text. Be concrete.\n\nCHAPTER EXCERPT:\n%s\n",
		12, text)
	return b.String()
}

// retryHint is appended when the previous response was not parseable JSON.
const retryHint = "Your previous response was not valid JSON. Respond again with ONLY the required JSON array, no markdown fences and no prose."
