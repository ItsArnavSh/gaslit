package corpus

import (
	"regexp"
	"strings"
)

var (
	pgStartRe = regexp.MustCompile(`(?i)\*{3}\s*START OF (?:THE|THIS) PROJECT GUTENBERG[^\n]*\*{3}`)
	pgEndRe   = regexp.MustCompile(`(?i)\*{3}\s*END OF (?:THE|THIS) PROJECT GUTENBERG[^\n]*\*{3}`)

	txtHeadingRe = regexp.MustCompile(`(?i)^\s*(chapter|book|part|section|canto|letter|essay|act|scene|volume|prologue|epilogue|introduction|preface|appendix)\b`)
	mdHeadingRe  = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+\S`)
)

// trimGutenbergBlocks removes Project Gutenberg license boilerplate from a
// block stream, dropping everything before the START marker and after the END
// marker when they are present.
func trimGutenbergBlocks(blocks []block) []block {
	start := -1
	for i, b := range blocks {
		if pgStartRe.MatchString(b.Text) {
			start = i + 1
		}
	}
	if start >= 0 {
		blocks = blocks[start:]
	}
	end := len(blocks)
	for i, b := range blocks {
		if pgEndRe.MatchString(b.Text) {
			end = i
			break
		}
	}
	return blocks[:end]
}

// parseText parses a plain-text (.txt/.md) document into a Source.
func parseText(name, origin string, data []byte) (*Source, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = trimGutenberg(text)

	blocks := textBlocks(text)
	chapters := BuildChapters(blocks, DefaultChapterOptions())
	return &Source{Name: name, Origin: origin, Format: "txt", Title: name, Chapters: chapters}, nil
}

// trimGutenberg strips Project Gutenberg license boilerplate when present.
func trimGutenberg(text string) string {
	if loc := pgStartRe.FindStringIndex(text); loc != nil {
		text = text[loc[1]:]
	}
	if loc := pgEndRe.FindStringIndex(text); loc != nil {
		text = text[:loc[0]]
	}
	return text
}

// textBlocks splits plain text into heading/paragraph blocks.
func textBlocks(text string) []block {
	lines := strings.Split(text, "\n")
	var blocks []block
	var para strings.Builder

	flush := func() {
		p := normalizeSpace(para.String())
		para.Reset()
		if p != "" {
			blocks = append(blocks, block{Text: p})
		}
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if lvl, title, ok := textHeading(trimmed); ok {
			flush()
			blocks = append(blocks, block{Heading: lvl, Text: title})
			continue
		}
		if para.Len() > 0 {
			para.WriteString(" ")
		}
		para.WriteString(trimmed)
	}
	flush()
	return blocks
}

// textHeading classifies a single line as a heading, returning its level.
func textHeading(line string) (int, string, bool) {
	if m := mdHeadingRe.FindStringSubmatch(line); m != nil {
		return len(m[1]), strings.TrimSpace(strings.TrimLeft(line, "# ")), true
	}
	if txtHeadingRe.MatchString(line) {
		return 1, line, true
	}
	if isAllCapsHeading(line) {
		return 1, line, true
	}
	return 0, "", false
}

// isAllCapsHeading reports whether a line looks like an ALL-CAPS heading: short,
// mostly letters, and not ending in sentence punctuation.
func isAllCapsHeading(line string) bool {
	if len(line) < 3 || len(line) > 60 {
		return false
	}
	letters, upper, hasDigit := 0, 0, false
	for _, r := range line {
		switch {
		case r >= 'a' && r <= 'z':
			letters++
		case r >= 'A' && r <= 'Z':
			letters++
			upper++
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if letters < 3 {
		return false
	}
	_ = hasDigit
	if strings.HasSuffix(line, ".") || strings.HasSuffix(line, ",") || strings.HasSuffix(line, ";") {
		return false
	}
	return upper*10 >= letters*9
}
