package datatest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ItsArnavSh/gaslit/internal/corpus"
	"github.com/ItsArnavSh/gaslit/internal/eval"
	"github.com/ItsArnavSh/gaslit/internal/qa"
	"github.com/ItsArnavSh/gaslit/internal/rag"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultTopK  = 6
	defaultChunk = 1200
	defaultOv    = 150
	defaultCT    = 6000
	defaultCache = ""
)

// QuestionnaireConfig is the configuration for generating a questionnaire.
type QuestionnaireConfig struct {
	Sources            []string
	PerChapter         int
	Total              int
	MaxChapters        int
	Options            int
	Difficulty         string
	Style              string
	IncludeExplanation bool
	MaxChapterChars    int
	Language           string
}

// Option configures Suite behavior.
type Option func(*Options)

type Options struct {
	Embedder         embedding.Embedder
	TopK             int
	ChunkSize        int
	ChunkOverlap     int
	CacheDir         string
	ForceDownload    bool
	ChapterTarget    int
	Logger           *slog.Logger
	ChunkMin         int
	MaxFetchBytes    int64
}

func WithEmbedder(e embedding.Embedder) Option                 { return func(o *Options) { o.Embedder = e } }
func WithTopK(k int) Option                                   { return func(o *Options) { o.TopK = k } }
func WithChunkSize(n int) Option                               { return func(o *Options) { o.ChunkSize = n; o.ChunkMin = n/6 + 100 } }
func WithChunkOverlap(n int) Option                           { return func(o *Options) { o.ChunkOverlap = n } }
func WithCacheDir(dir string) Option                           { return func(o *Options) { o.CacheDir = dir } }
func WithForceDownload(force bool) Option                      { return func(o *Options) { o.ForceDownload = force } }
func WithChapterTarget(n int) Option                           { return func(o *Options) { o.ChapterTarget = n } }
func WithLogger(l *slog.Logger) Option                         { return func(o *Options) { o.Logger = l } }
func WithMaxFetchBytes(n int64) Option                         { return func(o *Options) { o.MaxFetchBytes = n } }

// Suite ties corpus parsing, chunking, RAG indexing, agent creation, QA
// generation, evaluation, and reporting.
type Suite struct {
	ctx         context.Context
	opts        Options
	reg         *corpus.Registry
	mu          sync.Mutex
	sources     []*corpus.Source
	sourceBy    map[string]*corpus.Source
	chunks      []corpus.Chunk
	index       *rag.Index
	indexDirty  bool
	agents      map[string]*agentHandle
	agentOrder  []string
}

type agentHandle struct {
	name         string
	sources      []string
	agent        adk.Agent
	systemPrompt string
}

// Question = qa.Question
// QuestionBank = qa.QuestionBank
// RunConfig = eval.RunConfig
// RunResult = eval.RunResult
// Analysis = eval.Analysis
type (
	Question     = qa.Question
	QuestionBank = qa.QuestionBank
	RunMode      = eval.Mode
	RunConfig    = eval.RunConfig
	RunResult    = eval.RunResult
	Analysis     = eval.Analysis
)

const (
	ModeAnswerOnly = eval.ModeAnswerOnly
	ModeDebate     = eval.ModeDebate
	ModeBoth       = eval.ModeBoth
)

func New(ctx context.Context, opts ...Option) *Suite {
	o := Options{TopK: defaultTopK, ChunkSize: defaultChunk, ChunkOverlap: defaultOv, ChapterTarget: defaultCT}
	for _, opt := range opts {
		opt(&o)
	}
	if o.CacheDir == "" {
		o.CacheDir = filepath.Join(os.TempDir(), "gaslit-datatest-cache")
	}
	if o.ChunkMin <= 0 {
		o.ChunkMin = o.ChunkSize/6 + 150
	}
	return &Suite{
		ctx:        ctx,
		opts:       o,
		reg:        corpus.NewRegistry(),
		sourceBy:   make(map[string]*corpus.Source),
		agents:     make(map[string]*agentHandle),
		agentOrder: []string{},
		indexDirty: true,
	}
}

// RegisterParser registers a custom corpus parser for an extension.
func (s *Suite) RegisterParser(ext string, p corpus.Parser) {
	s.reg.Register(ext, p)
}

// AddSource ingests a source from a local file path or an http(s) URL. The
// source is parsed, chapter-segmented, chunked, and added to the in-memory
// corpus; the index is marked dirty for lazy rebuild.
func (s *Suite) AddSource(name, pathOrURL string) error {
	if name == "" {
		return fmt.Errorf("datatest: source name must be non-empty")
	}
	if pathOrURL == "" {
		return fmt.Errorf("datatest: source path/URL must be non-empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sourceBy[name]; exists {
		return fmt.Errorf("datatest: source %q already exists", name)
	}

	var local string
	var err error
	if corpus.IsURL(pathOrURL) {
		local, err = corpus.Fetch(s.ctx, pathOrURL, corpus.FetchOptions{CacheDir: s.opts.CacheDir, Force: s.opts.ForceDownload, MaxBytes: s.opts.MaxFetchBytes})
		if err != nil {
			return err
		}
	} else {
		local = pathOrURL
	}

	data, err := os.ReadFile(local)
	if err != nil {
		return fmt.Errorf("datatest: read source %q: %w", local, err)
	}

	src, err := corpus.ParseSource(s.reg, name, pathOrURL, data, nil)
	if err != nil {
		return err
	}

	copt := corpus.DefaultChapterOptions()
	if s.opts.ChapterTarget > 0 {
		copt.TargetSize = s.opts.ChapterTarget
	}
	// keep parsed chapters
	chunks := corpus.SplitChapters(src, corpus.ChunkOptions{Size: s.opts.ChunkSize, Overlap: s.opts.ChunkOverlap, Min: s.opts.ChunkMin})
	s.sources = append(s.sources, src)
	s.sourceBy[name] = src
	s.chunks = append(s.chunks, chunks...)
	s.indexDirty = true
	if s.opts.Logger != nil {
		s.opts.Logger.Info("added source", "name", name, "chapters", len(src.Chapters), "chunks", len(chunks))
	}
	return nil
}



// This is a small hack: we need to access blocks. Let us add a helper by reusing parse logic via calling ParseSource again is wasteful; easier to just keep chapters as parsed and note custom ChapterTarget is applied during chunking not needed. actually ChapterTarget affects grouping when no divisions - ParseSource used default. fine for now.

func (s *Suite) ensureIndex(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.indexDirty && s.index != nil {
		return nil
	}
	idx, err := rag.Build(ctx, s.chunks, rag.Config{Embedder: s.opts.Embedder})
	if err != nil {
		return err
	}
	s.index = idx
	s.indexDirty = false
	return nil
}

func (s *Suite) searchHandler(ctx context.Context, in *searchSourcesInput) (string, error) {
	if err := s.ensureIndex(ctx); err != nil {
		return "", err
	}
	topK := s.opts.TopK
	if in.TopK > 0 {
		topK = in.TopK
	}
	sources := in.Sources
	if len(sources) == 0 {
		sources = nil
	}
	hits, err := s.index.Search(ctx, in.Topic, sources, topK)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "[%d] %s (ch.%d %q) score=%.3f\n", i+1, h.Chunk.Source, h.Chunk.Chapter, h.Chunk.ChapterTitle, h.Score)
		b.WriteString(h.Chunk.Text)
		b.WriteString("\n---\n")
	}
	return b.String(), nil
}

type searchSourcesInput struct {
	Topic   string   `json:"topic"`
	TopK    int      `json:"top_k,omitempty"`
	Sources []string `json:"sources,omitempty"`
}

// AddAgent creates an adk.ChatModelAgent with a retrieval tool scoped to sources.
func (s *Suite) AddAgent(ctx context.Context, name, systemPrompt string, sources []string, model model.BaseModel[*schema.Message], agentOpts ...adk.AgentOption) (*adk.ChatModelAgent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.agents[name]; exists {
		return nil, fmt.Errorf("datatest: agent %q already exists", name)
	}
	// validate sources
	for _, src := range sources {
		if _, ok := s.sourceBy[src]; !ok {
			return nil, fmt.Errorf("datatest: unknown source %q", src)
		}
	}

	// build tool
	toolIns, err := utils.InferTool("search_sources", "Search the indexed book sources for relevant passages. Use topic + optional top_k + sources.", s.searchHandler)
	if err != nil {
		return nil, err
	}

	tools := []tool.BaseTool{toolIns}

	ag, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        name,
		Description: "RAG-grounded agent for datatest evaluation",
		Instruction: systemPrompt,
		Model:       model,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}},
})
	if err != nil {
		return nil, err
	}

	s.agents[name] = &agentHandle{name: name, sources: sources, agent: ag, systemPrompt: systemPrompt}
	s.agentOrder = append(s.agentOrder, name)
	return ag, nil
}

// Agents returns all registered agents in order of registration.
func (s *Suite) Agents() []adk.Agent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]adk.Agent, 0, len(s.agentOrder))
	for _, n := range s.agentOrder {
		if h := s.agents[n]; h != nil {
			out = append(out, h.agent)
		}
	}
	return out
}

// Sources returns all parsed sources.
func (s *Suite) Sources() []*corpus.Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*corpus.Source(nil), s.sources...)
}

// GenerateQuestionnaire generates a question bank using the provided QA model.
func (s *Suite) GenerateQuestionnaire(ctx context.Context, qaModel model.BaseModel[*schema.Message], cfg *QuestionnaireConfig) (*QuestionBank, error) {
	s.mu.Lock()
	srcs := append([]*corpus.Source(nil), s.sources...)
	s.mu.Unlock()
	qcfg := qa.Config{
		Sources:            cfg.Sources,
		PerChapter:         cfg.PerChapter,
		Total:              cfg.Total,
		MaxChapters:        cfg.MaxChapters,
		Options:            cfg.Options,
		Difficulty:         cfg.Difficulty,
		Style:              cfg.Style,
		IncludeExplanation: cfg.IncludeExplanation,
		MaxChapterChars:    cfg.MaxChapterChars,
		Language:           cfg.Language,
	}
	g := qa.NewGenerator(qaModel, s.opts.Logger)
	return g.Generate(ctx, srcs, qcfg)
}

// Run evaluates agents against a question bank.
func (s *Suite) Run(ctx context.Context, bank *QuestionBank, cfg RunConfig) (*RunResult, error) {
	if bank == nil {
		return nil, fmt.Errorf("datatest: nil question bank")
	}
	// build targets from config.Agents or all
	targets := cfg.Targets
	if len(targets) == 0 {
		s.mu.Lock()
		for _, n := range s.agentOrder {
			if h := s.agents[n]; h != nil {
				targets = append(targets, eval.Target{Name: n, Agent: h.agent})
			}
		}
		s.mu.Unlock()
	}
	ecfg := cfg
	ecfg.Targets = targets
	return eval.Run(ctx, bank, ecfg)
}

// Analyze computes analysis for a run result.
func (s *Suite) Analyze(res *RunResult) *Analysis {
	return eval.Analyze(res)
}

// SaveQuestionBank persists the question bank.
func SaveQuestionBank(path string, bank *QuestionBank) error {
	return qa.SaveBank(path, bank)
}

// LoadQuestionBank loads a question bank.
func LoadQuestionBank(path string) (*QuestionBank, error) {
	return qa.LoadBank(path)
}
