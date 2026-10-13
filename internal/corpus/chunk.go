package corpus

import (
	"fmt"
	"regexp"
	"strings"
)

// ChunkOptions controls how chapters are sliced for retrieval.
type ChunkOptions struct {
	// Size is the target chunk length in characters. Default 1200.
	Size int
	// Overlap is the number of trailing characters carried into the next chunk
	// for context continuity. Default 150.
	Overlap int
	// Min is the minimum chunk length; trailing fragments shorter than this are
	// merged into the previous chunk. Default 200.
	Min int
}

// DefaultChunkOptions returns the recommended chunk settings.
func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{Size: 1200, Overlap: 150, Min: 200}
}

func (o ChunkOptions) resolve() ChunkOptions {
	if o.Size <= 0 {
		o.Size = 1200
	}
	if o.Overlap < 0 {
		o.Overlap = 0
	}
	if o.Overlap >= o.Size {
		o.Overlap = o.Size / 5
	}
	if o.Min <= 0 {
		o.Min = 200
	}
	return o
}

// SplitChapters slices every chapter of a source into retrieval chunks.
func SplitChapters(src *Source, opt ChunkOptions) []Chunk {
	opt = opt.resolve()
	var chunks []Chunk
	for _, ch := range src.Chapters {
		parts := splitText(ch.Text, opt)
		for i, p := range parts {
			chunks = append(chunks, Chunk{
				ID:           fmt.Sprintf("%s:ch%d:%d", src.Name, ch.Index, i),
				Source:       src.Name,
				Chapter:      ch.Index,
				ChapterTitle: ch.Title,
				Index:        i,
				Text:         p,
			})
		}
	}
	return chunks
}

var sentenceRe = regexp.MustCompile(`(?m)[.!?]["')\]]?\s`)

// splitText splits a chapter into overlapping, paragraph-aware chunks. It
// prefers paragraph boundaries and falls back to sentences when a single
// paragraph exceeds the target size.
func splitText(text string, opt ChunkOptions) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= opt.Size {
		return []string{text}
	}

	paragraphs := strings.Split(text, "\n\n")
	var units []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) <= opt.Size {
			units = append(units, p)
			continue
		}
		units = append(units, splitSentences(p, opt.Size)...)
	}

	var chunks []string
	var cur strings.Builder
	flush := func() {
		s := strings.TrimSpace(cur.String())
		cur.Reset()
		if s != "" {
			chunks = append(chunks, s)
		}
	}

	for _, u := range units {
		if cur.Len() > 0 && cur.Len()+len(u)+2 > opt.Size {
			prev := cur.String()
			flush()
			if opt.Overlap > 0 {
				cur.WriteString(tail(prev, opt.Overlap))
			}
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(u)
	}
	flush()

	// Merge a short trailing fragment into the previous chunk.
	if len(chunks) > 1 && len(chunks[len(chunks)-1]) < opt.Min {
		last := chunks[len(chunks)-1]
		chunks = chunks[:len(chunks)-1]
		chunks[len(chunks)-1] = strings.TrimSpace(chunks[len(chunks)-1] + "\n\n" + last)
	}
	return chunks
}

// splitSentences packs sentences into pieces no longer than max.
func splitSentences(p string, max int) []string {
	locs := sentenceRe.FindAllStringIndex(p, -1)
	var sentences []string
	start := 0
	for _, loc := range locs {
		sentences = append(sentences, p[start:loc[1]])
		start = loc[1]
	}
	if start < len(p) {
		sentences = append(sentences, p[start:])
	}

	var out []string
	var cur strings.Builder
	for _, s := range sentences {
		if cur.Len() > 0 && cur.Len()+len(s) > max {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		cur.WriteString(s)
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

// tail returns the last n runes of s, trimmed to start at a word boundary.
func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	t := string(r[len(r)-n:])
	if i := strings.IndexByte(t, ' '); i >= 0 && i < len(t)-1 {
		t = t[i+1:]
	}
	return t
}
