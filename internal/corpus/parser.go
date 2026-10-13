package corpus

import (
	"fmt"
	"path"
	"strings"
)

// Parser converts raw bytes of a document into a Source. Implementations are
// responsible for detecting chapters; the chunker is applied afterwards.
type Parser interface {
	Parse(name, origin string, data []byte) (*Source, error)
}

// ParserFunc adapts a function to the Parser interface.
type ParserFunc func(name, origin string, data []byte) (*Source, error)

// Parse implements Parser.
func (f ParserFunc) Parse(name, origin string, data []byte) (*Source, error) {
	return f(name, origin, data)
}

// Registry maps a lowercase file extension (with leading dot, e.g. ".epub") to
// a Parser. It is consulted after the built-in text and EPUB parsers decline.
type Registry struct {
	parsers map[string]Parser
}

// NewRegistry returns a Registry preloaded with the built-in parsers.
func NewRegistry() *Registry {
	return &Registry{parsers: map[string]Parser{
		".txt":  ParserFunc(parseText),
		".text": ParserFunc(parseText),
		".md":   ParserFunc(parseText),
		".epub": ParserFunc(parseEPUB),
	}}
}

// Register installs (or replaces) the parser for an extension. ext may be given
// with or without a leading dot; it is normalized to include one.
func (r *Registry) Register(ext string, p Parser) {
	if p == nil {
		return
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	r.parsers[strings.ToLower(ext)] = p
}

// Lookup returns the parser for an extension, if any.
func (r *Registry) Lookup(ext string) (Parser, bool) {
	p, ok := r.parsers[strings.ToLower(ext)]
	return p, ok
}

// Extension returns the lowercase file extension (including the dot) for a path
// or URL, ignoring query strings and fragments.
func Extension(location string) string {
	loc := location
	if i := strings.IndexAny(loc, "?#"); i >= 0 {
		loc = loc[:i]
	}
	return strings.ToLower(path.Ext(loc))
}

// ParseSource parses data for a source using the registry, dispatching on the
// location's extension. extra parsers, when supplied, are registered on top of
// the built-in set.
func ParseSource(reg *Registry, name, origin string, data []byte, extra map[string]Parser) (*Source, error) {
	if reg == nil {
		reg = NewRegistry()
	}
	for ext, p := range extra {
		reg.Register(ext, p)
	}
	ext := Extension(origin)
	if ext == "" {
		ext = Extension(name)
	}
	p, ok := reg.Lookup(ext)
	if !ok {
		return nil, fmt.Errorf("corpus: no parser registered for %q (supported: .txt, .epub; register a Parser for anything else, e.g. PDF)", ext)
	}
	src, err := p.Parse(name, origin, data)
	if err != nil {
		return nil, err
	}
	if src.Format == "" {
		src.Format = strings.TrimPrefix(ext, ".")
	}
	if src.Title == "" {
		src.Title = name
	}
	src.Chars = src.TotalChars()
	return src, nil
}
