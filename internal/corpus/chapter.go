package corpus

import (
	"fmt"
	"regexp"
	"strings"
)

// ChapterOptions controls chapter segmentation.
type ChapterOptions struct {
	// TargetSize is the approximate character budget for a grouped chapter when
	// a source has no structural headings. Default 6000.
	TargetSize int
	// MinSize is the minimum character length of a chapter; shorter chapters are
	// merged into the preceding one so stray headings do not create stub
	// chapters. Default 600.
	MinSize int
}

// DefaultChapterOptions returns the recommended chapter settings.
func DefaultChapterOptions() ChapterOptions {
	return ChapterOptions{TargetSize: 6000, MinSize: 600}
}

func (o ChapterOptions) resolve() ChapterOptions {
	if o.TargetSize <= 0 {
		o.TargetSize = 6000
	}
	if o.MinSize <= 0 {
		o.MinSize = 600
	}
	return o
}

// divisionRe matches headings that denote a major structural division.
var divisionRe = regexp.MustCompile(`(?i)^\s*(book|chapter|part|section|canto|letter|essay|act|scene|volume|epistle|treatise|discourse|lecture)\b`)

// numericHeadingRe matches headings that are just a number or roman numeral,
// e.g. "1." or "XIV" — these are paragraph markers, not divisions.
var numericHeadingRe = regexp.MustCompile(`(?i)^\s*[0-9ivxlcdm]+[.)]?\s*$`)

// isDivision reports whether a heading marks a chapter boundary.
func isDivision(h block) bool {
	if h.Heading <= 0 {
		return false
	}
	text := strings.TrimSpace(h.Text)
	if text == "" || numericHeadingRe.MatchString(text) {
		return false
	}
	if divisionRe.MatchString(text) {
		return true
	}
	return h.Heading == 1
}

// BuildChapters turns a flat block stream into chapters. It splits on
// structural headings when present and otherwise groups blocks into
// roughly TargetSize-sized chapters.
func BuildChapters(blocks []block, opt ChapterOptions) []Chapter {
	opt = opt.resolve()

	divIdx := make([]int, 0)
	for i, b := range blocks {
		if isDivision(b) {
			divIdx = append(divIdx, i)
		}
	}

	var chapters []Chapter
	if len(divIdx) == 0 {
		chapters = groupBlocks(blocks, opt.TargetSize)
	} else {
		// Content before the first division becomes its own chapter.
		if divIdx[0] > 0 {
			if text := blocksToText(blocks[:divIdx[0]]); text != "" {
				chapters = append(chapters, Chapter{Title: "Front Matter", Text: text})
			}
		}
		for j, start := range divIdx {
			end := len(blocks)
			if j+1 < len(divIdx) {
				end = divIdx[j+1]
			}
			title := strings.TrimSpace(blocks[start].Text)
			body := blocksToText(blocks[start+1 : end])
			if body == "" {
				continue
			}
			chapters = append(chapters, Chapter{Title: title, Text: body})
		}
	}

	chapters = mergeSmall(chapters, opt.MinSize)
	for i := range chapters {
		chapters[i].Index = i
	}
	return chapters
}

func groupBlocks(blocks []block, target int) []Chapter {
	var chapters []Chapter
	var cur strings.Builder
	flush := func() {
		text := strings.TrimSpace(cur.String())
		cur.Reset()
		if text == "" {
			return
		}
		chapters = append(chapters, Chapter{
			Title: fmt.Sprintf("Part %d", len(chapters)+1),
			Text:  text,
		})
	}
	for _, b := range blocks {
		piece := blocksToText([]block{b})
		if piece == "" {
			continue
		}
		cur.WriteString(piece)
		if cur.Len() >= target {
			flush()
		}
	}
	flush()
	return chapters
}

// blocksToText renders blocks as paragraphs, with heading lines kept inline as
// short lines and paragraphs separated by blank lines.
func blocksToText(blocks []block) string {
	var b strings.Builder
	for i, blk := range blocks {
		if blk.Text == "" {
			continue
		}
		if i > 0 {
			if blk.Heading > 0 || blocks[i-1].Heading > 0 {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}
		b.WriteString(blk.Text)
	}
	return strings.TrimSpace(b.String())
}

// mergeSmall folds chapters shorter than min into a neighbour so stray headings
// do not produce stub chapters.
func mergeSmall(chapters []Chapter, min int) []Chapter {
	if len(chapters) == 0 {
		return chapters
	}
	out := make([]Chapter, 0, len(chapters))
	for _, c := range chapters {
		if len(c.Text) < min && len(out) > 0 {
			out[len(out)-1].Text = strings.TrimSpace(out[len(out)-1].Text + "\n\n" + c.Text)
			continue
		}
		out = append(out, c)
	}
	if len(out) > 1 && len(out[0].Text) < min {
		out[1].Text = strings.TrimSpace(out[0].Text + "\n\n" + out[1].Text)
		out = out[1:]
	}
	return out
}
