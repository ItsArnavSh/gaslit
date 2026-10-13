// Package corpus loads, parses, chapter-segments, and chunks book sources for
// the datatest component. It supports plain text and EPUB out of the box and
// can fetch sources over HTTP(S) with an on-disk cache. PDF is intentionally
// not built in; callers can register a Parser for it.
package corpus

// Source is a parsed book: its name, where it came from, its format, and the
// chapters detected in it.
type Source struct {
	Name     string    `json:"name"`
	Origin   string    `json:"origin"`
	Format   string    `json:"format"`
	Title    string    `json:"title,omitempty"`
	Chars    int       `json:"chars"`
	Chapters []Chapter `json:"chapters"`
}

// Chapter is a contiguous division of a source (a "Book", "Chapter", or a
// size-bounded group when no structural headings exist).
type Chapter struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Chunk is a retrieval-sized slice of a chapter. It carries enough metadata to
// trace a retrieved passage back to its source and chapter.
type Chunk struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Chapter      int    `json:"chapter"`
	ChapterTitle string `json:"chapter_title"`
	Index        int    `json:"index"`
	Text         string `json:"text"`
}

// TotalChars returns the combined length of every chapter in the source.
func (s *Source) TotalChars() int {
	n := 0
	for i := range s.Chapters {
		n += len(s.Chapters[i].Text)
	}
	return n
}
