package rag

import (
	"context"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// Metadata keys attached to retrieved documents.
const (
	MetaSource       = "source"
	MetaChapter      = "chapter"
	MetaChapterTitle = "chapter_title"
	MetaChunkIndex   = "chunk_index"
	MetaChunkID      = "chunk_id"
	MetaScore        = "score"
)

// Retriever adapts an Index to the eino retriever.Retriever interface.
type Retriever struct {
	Index *Index
	// Sources optionally restricts retrieval to a set of source names.
	Sources []string
}

// Retrieve implements retriever.Retriever.
func (r *Retriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(&retriever.Options{TopK: intPtr(6)}, opts...)
	topK := 6
	if options.TopK != nil {
		topK = *options.TopK
	}
	hits, err := r.Index.Search(ctx, query, r.Sources, topK)
	if err != nil {
		return nil, err
	}
	docs := make([]*schema.Document, 0, len(hits))
	for _, h := range hits {
		if options.ScoreThreshold != nil && h.Score < *options.ScoreThreshold {
			continue
		}
		docs = append(docs, h.Document())
	}
	return docs, nil
}

// Document converts a result into a schema.Document with provenance metadata.
func (r Result) Document() *schema.Document {
	return &schema.Document{
		ID:      r.Chunk.ID,
		Content: r.Chunk.Text,
		MetaData: map[string]any{
			MetaSource:       r.Chunk.Source,
			MetaChapter:      r.Chunk.Chapter,
			MetaChapterTitle: r.Chunk.ChapterTitle,
			MetaChunkIndex:   r.Chunk.Index,
			MetaChunkID:      r.Chunk.ID,
			MetaScore:        r.Score,
		},
	}
}

func intPtr(v int) *int { return &v }
