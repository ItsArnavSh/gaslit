package engine

import (
	"context"
	"testing"
	"time"

	"github.com/ItsArnavSh/gaslit/internal/core"
	"github.com/ItsArnavSh/gaslit/internal/scoring"
	"github.com/cloudwego/eino/adk"
)

func TestNewValidation(t *testing.T) {
	t.Run("requires supervisor model", func(t *testing.T) {
		cfg := core.PresetDefault()
		cfg.Agents = []adk.Agent{&fakeAgent{name: "a"}}
		if _, err := New(context.Background(), cfg); err == nil {
			t.Fatal("expected error for missing SupervisorModel")
		}
	})

	t.Run("requires agents", func(t *testing.T) {
		cfg := core.PresetDefault()
		cfg.SupervisorModel = stubModel{}
		if _, err := New(context.Background(), cfg); err == nil {
			t.Fatal("expected error for missing Agents")
		}
	})

	t.Run("rejects duplicate agent names", func(t *testing.T) {
		cfg := testConfig()
		cfg.Agents = []adk.Agent{
			&fakeAgent{name: "dup"},
			&fakeAgent{name: "dup"},
		}
		if _, err := New(context.Background(), cfg); err == nil {
			t.Fatal("expected error for duplicate agent names")
		}
	})

	t.Run("rejects empty agent name", func(t *testing.T) {
		cfg := testConfig()
		cfg.Agents = []adk.Agent{&fakeAgent{name: ""}}
		if _, err := New(context.Background(), cfg); err == nil {
			t.Fatal("expected error for empty agent name")
		}
	})
}

func TestResolveDefaults(t *testing.T) {
	cfg := core.PresetDefault()
	cfg.SupervisorModel = stubModel{}
	cfg.Agents = []adk.Agent{&fakeAgent{name: "a"}}
	resolved, err := resolveConfig(cfg)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	cfg = resolved

	if cfg.MaxRounds != 1 {
		t.Errorf("MaxRounds default = %d, want 1", cfg.MaxRounds)
	}
	if cfg.Concurrency != 1 {
		t.Errorf("Concurrency default = %d, want 1", cfg.Concurrency)
	}
	if cfg.MaxRetries != 2 {
		t.Errorf("MaxRetries default = %d, want 2", cfg.MaxRetries)
	}
	if cfg.Thresholds.DriftSignificance != 0.15 {
		t.Errorf("DriftSignificance default = %v, want 0.15", cfg.Thresholds.DriftSignificance)
	}
	if cfg.Thresholds.Confidence != 0.55 {
		t.Errorf("Confidence default = %v, want 0.55", cfg.Thresholds.Confidence)
	}
	if cfg.Thresholds.Consistency != 0.60 {
		t.Errorf("Consistency default = %v, want 0.60", cfg.Thresholds.Consistency)
	}
	if cfg.Protocol != core.ProtocolSupervised {
		t.Errorf("Protocol default = %v, want supervised", cfg.Protocol)
	}
	if cfg.GrillStrategy != core.GrillSingleSuspect {
		t.Errorf("GrillStrategy default = %v, want single_suspect", cfg.GrillStrategy)
	}
	if _, ok := cfg.AnswerSchema.(*core.MCQSchema); !ok {
		t.Errorf("AnswerSchema default = %T, want *core.MCQSchema", cfg.AnswerSchema)
	}
	if cfg.DriftMetric == nil {
		t.Error("DriftMetric default missing")
	}
	if cfg.ConsistencyModel == nil {
		t.Error("ConsistencyModel default missing")
	}
	if cfg.ConsistencyScorer == nil {
		t.Error("ConsistencyScorer default missing")
	}
	if scorer, ok := cfg.ConsistencyScorer.(*scoring.LLMScorer); ok {
		if scorer.Model != cfg.ConsistencyModel {
			t.Error("LLM scorer model does not match ConsistencyModel")
		}
	}
	if cfg.PromptTemplates.InitialPrompt == nil || cfg.PromptTemplates.GrillProbe == nil {
		t.Error("PromptTemplates defaults not populated")
	}
}

// cheapModel is a distinct model type used to verify the consistency scorer can
// be pointed at a different model than the supervisor.
type cheapModel struct{ stubModel }

func TestConsistencyModelSplit(t *testing.T) {
	cfg := testConfig()
	cfg.ConsistencyScorer = nil // exercise the resolveConfig default
	core.WithConsistencyModel(cheapModel{})(&cfg)
	resolved, err := resolveConfig(cfg)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.ConsistencyModel == resolved.SupervisorModel {
		t.Error("ConsistencyModel should be independent of SupervisorModel")
	}
	scorer, ok := resolved.ConsistencyScorer.(*scoring.LLMScorer)
	if !ok {
		t.Fatalf("ConsistencyScorer = %T, want *scoring.LLMScorer", resolved.ConsistencyScorer)
	}
	if scorer.Model != resolved.ConsistencyModel {
		t.Error("LLM scorer model does not match the configured ConsistencyModel")
	}
}

func TestScorerOptions(t *testing.T) {
	cfg := testConfig()
	WithLexicalScorer()(&cfg)
	if _, ok := cfg.ConsistencyScorer.(*scoring.LexicalScorer); !ok {
		t.Fatalf("WithLexicalScorer set ConsistencyScorer to %T, want *scoring.LexicalScorer", cfg.ConsistencyScorer)
	}

	cfg = testConfig()
	WithLLMScorer(cheapModel{})(&cfg)
	scorer, ok := cfg.ConsistencyScorer.(*scoring.LLMScorer)
	if !ok {
		t.Fatalf("WithLLMScorer set ConsistencyScorer to %T, want *scoring.LLMScorer", cfg.ConsistencyScorer)
	}
	if scorer.Model == nil {
		t.Error("WithLLMScorer left the scorer model nil")
	}
}

func TestPresets(t *testing.T) {
	strict := core.PresetStrict()
	if strict.Thresholds.DriftSignificance != 0.10 {
		t.Errorf("strict drift threshold = %v, want 0.10", strict.Thresholds.DriftSignificance)
	}
	lenient := core.PresetLenient()
	if lenient.Thresholds.DriftSignificance != 0.25 {
		t.Errorf("lenient drift threshold = %v, want 0.25", lenient.Thresholds.DriftSignificance)
	}
}

func TestOptions(t *testing.T) {
	g := rawEngine(t)

	co := g.resolveCall(core.WithMaxRoundsCall(3), core.WithVerboseCall(true))
	if co.MaxRounds != 3 {
		t.Errorf("MaxRounds = %d, want 3", co.MaxRounds)
	}
	if !co.Verbose {
		t.Error("Verbose not set")
	}

	co2 := g.resolveCall(core.WithThresholdsCall(core.Thresholds{DriftSignificance: 0.5, Confidence: 0.9, Consistency: 0.9}))
	if co2.Thresholds.DriftSignificance != 0.5 {
		t.Errorf("DriftSignificance = %v, want 0.5", co2.Thresholds.DriftSignificance)
	}
}

func TestWithPacingDelayOption(t *testing.T) {
	g := rawEngine(t, func(c *core.Config) { c.PacingDelay = 0 })
	// Setting via option then building should apply; verify option helper works.
	cfg := testConfig()
	core.WithPacingDelay(5 * time.Second)(&cfg)
	if cfg.PacingDelay != 5*time.Second {
		t.Errorf("PacingDelay = %v, want 5s", cfg.PacingDelay)
	}
	_ = g
}
