// Package rag is a small, dependency-light retrieval layer for the datatest
// component. It builds an in-memory index over corpus chunks with BM25 by
// default and optional dense embeddings, and exposes the result through the
// eino retriever.Retriever interface as well as a source-filtered Search.
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/ItsArnavSh/gaslit/internal/corpus"
	"github.com/cloudwego/eino/components/embedding"
)

// Config configures index construction and scoring.
type Config struct {
	// K1 and B are the BM25 saturation and length-normalization parameters.
	// Zero values default to 1.5 and 0.75.
	K1 float64
	B  float64
	// Embedder, when set, enables dense retrieval over chunk embeddings.
	Embedder embedding.Embedder
}

func (c Config) resolve() Config {
	if c.K1 <= 0 {
		c.K1 = 1.5
	}
	if c.B < 0 || c.B > 1 {
		c.B = 0.75
	}
	if c.B == 0 {
		c.B = 0.75
	}
	return c
}

// Index is an immutable retrieval index over a slice of chunks.
type Index struct {
	chunks   []corpus.Chunk
	bySource map[string][]int
	postings map[string]map[int]int
	df       map[string]int
	docLen   []int
	avgDL    float64
	vectors  [][]float64
	embedder embedding.Embedder
	k1       float64
	b        float64
}

// Result is a single scored retrieval hit.
type Result struct {
	Chunk corpus.Chunk
	Score float64
}

// Build tokenizes chunks, computes BM25 statistics, and (when configured)
// embeds every chunk for dense retrieval.
func Build(ctx context.Context, chunks []corpus.Chunk, cfg Config) (*Index, error) {
	cfg = cfg.resolve()
	ix := newIndex(chunks, cfg, nil)

	if cfg.Embedder != nil && len(chunks) > 0 {
		vecs, err := embedAll(ctx, cfg.Embedder, chunkTexts(chunks))
		if err != nil {
			return nil, fmt.Errorf("rag: embed chunks: %w", err)
		}
		ix.vectors = normalizeAll(vecs)
	}
	return ix, nil
}

// newIndex builds the lexical structures, optionally seeding dense vectors.
func newIndex(chunks []corpus.Chunk, cfg Config, vectors [][]float64) *Index {
	ix := &Index{
		chunks:   append([]corpus.Chunk(nil), chunks...),
		bySource: make(map[string][]int),
		postings: make(map[string]map[int]int),
		df:       make(map[string]int),
		docLen:   make([]int, len(chunks)),
		vectors:  vectors,
		embedder: cfg.Embedder,
		k1:       cfg.K1,
		b:        cfg.B,
	}
	total := 0
	for i, c := range chunks {
		ix.bySource[c.Source] = append(ix.bySource[c.Source], i)
		toks := tokenize(c.Text)
		ix.docLen[i] = len(toks)
		total += len(toks)
		seen := make(map[string]bool, len(toks))
		for _, t := range toks {
			m := ix.postings[t]
			if m == nil {
				m = make(map[int]int)
				ix.postings[t] = m
			}
			m[i]++
			if !seen[t] {
				seen[t] = true
				ix.df[t]++
			}
		}
	}
	if len(chunks) > 0 {
		ix.avgDL = float64(total) / float64(len(chunks))
	}
	return ix
}

// Len returns the number of indexed chunks.
func (ix *Index) Len() int { return len(ix.chunks) }

// Sources returns the source names present in the index.
func (ix *Index) Sources() []string {
	out := make([]string, 0, len(ix.bySource))
	for s := range ix.bySource {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Search returns the topK most relevant chunks, optionally restricted to a set
// of source names. It uses dense cosine similarity when an embedder is
// configured, otherwise BM25.
func (ix *Index) Search(ctx context.Context, query string, sources []string, topK int) ([]Result, error) {
	if topK <= 0 {
		topK = 6
	}
	candidates := ix.candidates(sources)

	var scored []Result
	if ix.embedder != nil && len(ix.vectors) == len(ix.chunks) {
		qv, err := ix.embedder.EmbedStrings(ctx, []string{query})
		if err != nil {
			return nil, fmt.Errorf("rag: embed query: %w", err)
		}
		if len(qv) == 1 {
			scored = ix.denseSearch(normalize(qv[0]), candidates)
		}
	}
	if scored == nil {
		scored = ix.bm25Search(query, candidates)
	}

	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > topK {
		scored = scored[:topK]
	}
	return scored, nil
}

// candidates returns the chunk indices to score, restricted to sources when
// non-empty, and nil when the whole index should be searched.
func (ix *Index) candidates(sources []string) []int {
	if len(sources) == 0 {
		return nil
	}
	var out []int
	for _, s := range sources {
		out = append(out, ix.bySource[s]...)
	}
	return out
}

// SearchAll scores every chunk provided by candidateSet (or the whole index).
func (ix *Index) forEachCandidate(candidateSet []int, fn func(i int)) {
	if candidateSet == nil {
		for i := range ix.chunks {
			fn(i)
		}
		return
	}
	for _, i := range candidateSet {
		fn(i)
	}
}

// bm25Search scores candidates with BM25 and returns all positive hits.
func (ix *Index) bm25Search(query string, candidateSet []int) []Result {
	terms := unique(tokenize(query))
	if len(terms) == 0 {
		return nil
	}
	n := float64(len(ix.chunks))
	scores := make(map[int]float64)
	ix.forEachCandidate(candidateSet, func(i int) {
		dl := float64(ix.docLen[i])
		var s float64
		for _, term := range terms {
			tf, ok := ix.postings[term][i]
			if !ok {
				continue
			}
			df := float64(ix.df[term])
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			denom := float64(tf) + ix.k1*(1-ix.b+ix.b*dl/ix.avgDL)
			s += idf * (float64(tf) * (ix.k1 + 1)) / denom
		}
		if s > 0 {
			scores[i] = s
		}
	})
	return mapToResults(ix.chunks, scores)
}

// denseSearch scores candidates by cosine similarity.
func (ix *Index) denseSearch(qv []float64, candidateSet []int) []Result {
	scores := make(map[int]float64)
	ix.forEachCandidate(candidateSet, func(i int) {
		if i >= len(ix.vectors) {
			return
		}
		if s := cosine(qv, ix.vectors[i]); s > 0 {
			scores[i] = s
		}
	})
	return mapToResults(ix.chunks, scores)
}

func mapToResults(chunks []corpus.Chunk, scores map[int]float64) []Result {
	out := make([]Result, 0, len(scores))
	for i, s := range scores {
		out = append(out, Result{Chunk: chunks[i], Score: s})
	}
	return out
}

func chunkTexts(chunks []corpus.Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Text
	}
	return out
}

// --- persistence ---

type snapshot struct {
	Version int             `json:"version"`
	Chunks  []corpus.Chunk  `json:"chunks"`
	Vectors [][]float64     `json:"vectors,omitempty"`
}

const snapshotVersion = 1

// Save writes the index to path as JSON.
func (ix *Index) Save(path string) error {
	snap := snapshot{Version: snapshotVersion, Chunks: ix.chunks, Vectors: ix.vectors}
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads an index written by Save. When an embedder is configured but the
// stored vectors are missing or stale, chunks are re-embedded.
func Load(ctx context.Context, path string, cfg Config) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("rag: decode index: %w", err)
	}
	cfg = cfg.resolve()

	vectors := snap.Vectors
	if cfg.Embedder != nil && len(vectors) != len(snap.Chunks) {
		vecs, err := embedAll(ctx, cfg.Embedder, chunkTexts(snap.Chunks))
		if err != nil {
			return nil, fmt.Errorf("rag: re-embed on load: %w", err)
		}
		vectors = normalizeAll(vecs)
	}
	return newIndex(snap.Chunks, cfg, vectors), nil
}

// --- tokenization ---

var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "but": true, "by": true, "for": true, "from": true, "has": true,
	"he": true, "her": true, "his": true, "i": true, "in": true, "is": true,
	"it": true, "its": true, "of": true, "on": true, "or": true, "that": true,
	"the": true, "their": true, "they": true, "this": true, "to": true, "was": true,
	"were": true, "which": true, "who": true, "will": true, "with": true, "you": true,
	"we": true, "not": true, "no": true, "do": true, "does": true, "did": true,
	"so": true, "if": true, "then": true, "than": true, "there": true, "been": true,
	"have": true, "had": true, "would": true, "could": true, "should": true, "may": true,
}

// tokenize lowercases and splits text into alphanumeric word tokens, dropping
// stopwords and single characters.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := fields[:0]
	for _, t := range fields {
		if len(t) < 2 || stopwords[t] {
			continue
		}
		out = append(out, t)
	}
	return out
}

func unique(toks []string) []string {
	seen := make(map[string]bool, len(toks))
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		if len(t) < 2 || stopwords[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// --- embedding helpers ---

func embedAll(ctx context.Context, e embedding.Embedder, texts []string) ([][]float64, error) {
	const batch = 64
	var out [][]float64
	for i := 0; i < len(texts); i += batch {
		end := i + batch
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := e.EmbedStrings(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func normalizeAll(vecs [][]float64) [][]float64 {
	out := make([][]float64, len(vecs))
	for i, v := range vecs {
		out[i] = normalize(v)
	}
	return out
}

func normalize(v []float64) []float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return v
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

func cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
