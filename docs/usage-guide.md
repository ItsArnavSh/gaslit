# Usage Guide

This guide walks through installing gaslit, building a panel of agents,
configuring the engine, running debates, reading the results, and using the
standalone functions.

---

## 1. Install

```sh
go get github.com/ItsArnavSh/gaslit
```

Requires **Go 1.26+**. gaslit is built on top of
[cloudwego/eino](https://github.com/cloudwego/eino) — your debaters and the
moderator are ordinary `adk.Agent`/`model.BaseModel` values, so anything that
works with eino works here.

## 2. Minimal working example

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ItsArnavSh/gaslit"
	"github.com/cloudwego/eino/adk"
)

func main() {
	ctx := context.Background()

	// Build debater agents (any eino adk.Agent).
	econ, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "economic_historian",
		Description: "Analyzes events from a socio-economic lens.",
		Instruction: "You are an economic historian. Argue that economic forces drive history.",
		Model:       myModel, // your model.BaseModel[*schema.Message]
	})
	pol, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "political_historian",
		Description: "Analyzes events through leadership and military strategy.",
		Instruction: "You are a political and military historian.",
		Model:       myModel,
	})

	cfg := gaslit.PresetDefault()
	cfg.SupervisorModel = myModel
	cfg.Agents = []adk.Agent{econ, pol}
	cfg.Concurrency = 2

	engine, err := gaslit.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}

	summary, err := engine.Debate(ctx,
		"What caused the Roman Empire's collapse?",
		[]string{"Economic exhaustion", "Military breakdown"},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Winner:  %s\n", summary.Winner)
	fmt.Printf("Drift:   %t (driver: %s)\n", summary.DriftVerdict.HasDrift, summary.DriftVerdict.PrimaryDriver)
	fmt.Printf("Outcome: %s\n", summary.OverallOutcome)
	for name, diag := range summary.Diagnostics {
		fmt.Printf("%s → %s (conf %.2f, consistency %.2f)\n",
			name, diag.Quadrant, diag.ConfidenceScore, diag.ConsistencyScore)
	}
}
```

See [`examples/basic`](../examples/basic) for a fully runnable version using
Groq's API (including a 429-backoff transport and a "panel of historians" demo,
one of which is deliberately instructed to hallucinate):

```sh
GROQ_API_KEY=... go run ./examples/basic          # debug: LexicalScorer, no NLI calls
GROQ_API_KEY=... go run ./examples/basic -final   # benchmark: cheap LLM NLI scorer
```

The example splits models (strong moderator, cheap consistency model), caps
each debate with a `CostBudget`, and demonstrates the
save → load → reclassify/rescore caching workflow.

## 3. Building the config

Construct a `Config` with a preset, then set fields directly or through
`Option` functions passed to `New`:

```go
cfg := gaslit.PresetDefault()          // or PresetStrict() / PresetLenient()
engine, err := gaslit.New(ctx, cfg,
	gaslit.WithPacingDelay(3*time.Second), // throttle calls (rate limits)
	gaslit.WithVerbose(true),
	gaslit.WithConcurrency(2),
	gaslit.WithGrillStrategy(gaslit.GrillAllDrifted),
)
```

### Required fields

| Field | Notes |
|---|---|
| `cfg.SupervisorModel` | The moderator model. **Required.** |
| `cfg.Agents` | The panel; ≥ 1 agent. Names (`Agent.Name(ctx)`) must be unique. |

Optionally, decouple the NLI/consistency check from the moderator:

| Field | Notes |
|---|---|
| `cfg.ConsistencyModel` | Powers the default `*LLMScorer`. Defaults to `SupervisorModel`. Use a **cheap** model here and a strong one for `SupervisorModel`. |

### Thresholds

`Thresholds{DriftSignificance, Confidence, Consistency}`, all in `[0,1]`.

- `DriftSignificance` — minimum shift/erosion to flag drift (default 0.15).
- `Confidence` — x-axis cut-off for quadrant classification (default 0.55).
- `Consistency` — y-axis cut-off for quadrant classification (default 0.60).

Presets set these for you: `PresetStrict` uses (0.10, 0.60, 0.65);
`PresetLenient` uses (0.25, 0.45, 0.50).

### Pluggable behavior

| Behavior | Options (defaults in bold) |
|---|---|
| Drift metric | **`TVDRift`**, `KLDistance`, `JSDistance`, `MaxChange`, or custom `DriftMetric` |
| Attribution | **`ChampionAttribution`**, `ShiftWeightedAttribution`, `CorrelationAttribution`, or custom `AttributionFn` |
| Classifier | **`ThresholdClassifier`**, or custom `Classifier` |
| Consistency scorer | **`*LLMScorer`** (uses `ConsistencyModel`), `*LexicalScorer` (deterministic, zero extra calls), or custom `ConsistencyScorer` |
| Aggregation | **`MajorityVote`**, `ConfidenceWeightedVote`, `WeightedVote`, `ConfidenceWeightedWeightedVote`, or custom `AggregationFn` |
| Protocol | **`ProtocolSupervised`**, `ProtocolAdversarialPairs`, `ProtocolRoundRobin`, `ProtocolDevilsAdvocate` |
| Grill strategy | **`GrillSingleSuspect`**, `GrillNone`, `GrillInfluencer`, `GrillAllDrifted`, `GrillTopN` (with `GrillCount`) |
| Answer schema | **`*MCQSchema`**, `*OpenEndedSchema`, or custom `AnswerSchema` |
| Prompts | Any template via `PromptTemplates` |

Example:

```go
cfg.Thresholds = gaslit.Thresholds{DriftSignificance: 0.15, Confidence: 0.55, Consistency: 0.60}
cfg.DriftMetric = gaslit.TVDRift
cfg.AttributionFn = gaslit.ChampionAttribution
cfg.GrillStrategy = gaslit.GrillSingleSuspect
cfg.Protocol = gaslit.ProtocolSupervised
cfg.ConsistencyScorer = &gaslit.LexicalScorer{} // skip extra model calls
```

### Debug vs. final-benchmark consistency scoring

`LexicalScorer` is a **weak proxy** for real semantic consistency — fine while
you're iterating on harness logic (question loading, scoring, quadrant tallying)
but not for final numbers. Split your models and toggle the scorer:

```go
// Debugging the pipeline: deterministic, zero extra model calls.
engine, _ := gaslit.New(ctx, cfg, gaslit.WithLexicalScorer())

// Final benchmark: cheap model does the NLI check, strong model debates.
cfg.ConsistencyModel = cheapModel          // or gaslit.WithConsistencyModel(cheapModel)
engine, _ = gaslit.New(ctx, cfg)           // default *LLMScorer now uses cheapModel
```

Or override the scorer per call without rebuilding the engine:

```go
engine.Debate(ctx, q, opts, gaslit.WithConsistencyScorerCall(&gaslit.LexicalScorer{}))
```

### Per-agent configuration

`WithAgentConfig(name, AgentConfig{...})` customizes a single panelist:

```go
gaslit.WithAgentConfig("economic_historian", gaslit.AgentConfig{
	Instruction: "Focus on fiscal policy and taxation.",
	Weight:      1.5,                       // used by weighted aggregation
	SkipPhases:  map[gaslit.Phase]bool{gaslit.PhaseGrilling: true},
})
```

`SkipPhases` lets an agent sit out specific phases entirely.

> **Always set a `Budget` for unattended runs.** `MaxCalls`/`MaxTokens` cap the
> debate up front, so a bug (like an infinite retry loop) surfaces as a
> `*gaslit.BudgetError` instead of burning your quota.

### Execution, budget, observability

```go
cfg.PacingDelay   = 500 * time.Millisecond // throttle each model call
cfg.RequestTimeout = 30 * time.Second      // per-call timeout
cfg.MaxRetries     = 2                     // re-prompts on unparseable JSON
cfg.Concurrency    = 4                     // parallel agent queries
cfg.MaxRounds      = 1                     // (cross-examine → evaluate) rounds

cfg.Budget = gaslit.CostBudget{
	MaxCalls:   500,               // model-invocation cap per debate
	MaxTokens:  100_000,           // approximate (chars/4) cap
	Deadline:   5 * time.Minute,   // wall-clock bound
}

cfg.Recorder = gaslit.NewJSONLRecorder(file) // persist event stream
cfg.Redact   = redactFunc                    // scrub PII from every outgoing prompt
cfg.Logger   = slog.New(...)
cfg.Hooks    = gaslit.Hooks{
	OnDrift: func(_ context.Context, v *gaslit.DriftVerdict) {
		fmt.Println("drift:", v.PrimaryDriver)
	},
}
```

### Monitoring

```go
stats := engine.Stats()
fmt.Println(stats.TotalCalls, stats.Tokens, stats.AgentCalls, stats.PhaseDuration)
```

## 4. Running debates

### Multiple choice

```go
summary, err := engine.Debate(ctx, question, []string{opt1, opt2, opt3, opt4})
```

Options are auto-labeled `A`, `B`, `C`, ... — or set `cfg.OptionLabels` /
`WithOptionLabelsCall`. The winning option (`summary.Winner`) is the label, not
the text.

### Open-ended

Pass an **empty** options slice. Agents answer in free text, the open-ended
schema is used automatically, and drift is measured lexically (Jaccard
distance) instead of over confidences.

```go
summary, err := engine.Debate(ctx, "Explain the long-term causes of the Cold War.", nil)
```

### Multi-round

```go
// engine-level
cfg.MaxRounds = 3

// or per-call
summary, err := engine.DebateN(ctx, question, options, 3)
summary, err = engine.Debate(ctx, question, options, gaslit.WithMaxRoundsCall(3))
```

### Batches

`DebateMany` runs a sequence of debates **reusing panel context**, so agents
carry knowledge across questions:

```go
summaries, err := engine.DebateMany(ctx,
	[]string{"Q1?", "Q2?", "Q3?"},
	[][]string{{"a", "b"}, {"a", "b", "c"}, nil},
)
```

`questions` and `optionsList` must be the same length.

## 5. Per-call options

`CallOption`s merge over the engine config for a single call — the engine is
not mutated and remains reusable:

```go
engine.Debate(ctx, question, options,
	gaslit.WithVerboseCall(true),
	gaslit.WithThresholdsCall(gaslit.Thresholds{DriftSignificance: 0.05}),
	gaslit.WithMaxRoundsCall(2),
	gaslit.WithMetadataCall(map[string]any{"run": "nightly"}),
	gaslit.WithBudgetCall(gaslit.CostBudget{MaxCalls: 100}),
)
```

Full list: `WithVerboseCall`, `WithMaxRoundsCall`, `WithOptionLabelsCall`,
`WithThresholdsCall`, `WithGrillStrategyCall`, `WithGrillCountCall`,
`WithProtocolCall`, `WithMetadataCall`, `WithConfidenceCall`, `WithClaimCall`,
`WithHooksCall`, `WithBudgetCall`.

## 6. Using the pipeline slices directly

Every stage is callable standalone. Example — run a debate manually and stream
each milestone:

```go
events, err := engine.Stream(ctx, question, options)
for ev := range events {
	switch ev.Type {
	case gaslit.EventBaseline:
		fmt.Printf("%s baseline: %s\n", ev.Agent, ev.Answer.Answer)
	case gaslit.EventDrift:
		fmt.Printf("drift verdict: %+v\n", *ev.Verdict)
	case gaslit.EventDiagnostic:
		fmt.Printf("%s → %s\n", ev.Diagnostic.AgentName, ev.Diagnostic.Quadrant)
	case gaslit.EventError:
		fmt.Println("error:", ev.Error)
	case gaslit.EventDone:
		fmt.Println("done")
	}
}
```

Sliced calls:

- `engine.Baseline(ctx, question, options)` → `map[string]*Answer`
- `engine.CrossExamine(ctx, question, before)`
- `engine.Evaluate(ctx, options)` → `map[string]*Answer`
- `engine.DriftAnalysis(before, after, options)` → `DriftVerdict`
- `engine.Grill(ctx, agentName, claim, question)` → `*DiagnosticResult`

## 7. Standalone CCI probe

`CCI` runs the confidence/consistency probe against **any** agent — even one
never registered in the panel. Useful for auditing a single model's claims.

```go
diag, err := engine.CCI(ctx, someAgent,
	"The pyramids were built by aliens.",
	"Who built the Egyptian pyramids?",
)
fmt.Println(diag.Quadrant, diag.Recommendation)
```

You can supply a known confidence instead of a self-report probe:

```go
engine.CCI(ctx, agent, stmt, question, gaslit.WithConfidenceCall(0.9))
```

## 8. Persistence

```go
// record every event as JSONL
f, _ := os.Create("debate.jsonl")
engine := /* New(...) with */ gaslit.WithRecorder(gaslit.NewJSONLRecorder(f))

// dump / reload a summary
gaslit.SaveSummary(os.Stdout, summary)
var loaded gaslit.FullDebateSummary
_ = json.NewDecoder(...) // or
```

`LoadSummary` is the round-trip reader for `SaveSummary` output. File variants
`SaveSummaryFile(path, s)` / `LoadSummaryFile(path)` write/read directly.

### Cache the debate, iterate on the rubric

If you're going to re-run diagnostics/scoring against the same question set,
don't re-call Baseline/Debate each time. Run it once, save it, then re-run only
your scoring/quadrant code against the saved JSON — your biggest saving, since
the rubric changes far more often than your question set.

```go
// Run once, cache.
gaslit.SaveSummaryFile("run.json", summary)

// Later: load, then re-tally quadrants with zero model calls.
loaded, _ := gaslit.LoadSummaryFile("run.json")
gaslit.ReclassifySummary(loaded, gaslit.Thresholds{Consistency: 0.65})

// Or: re-run only the consistency scorer over the stored R1/R2 (no new debate).
// Deterministic pass:
engine.Rescore(ctx, loaded, gaslit.WithConsistencyScorerCall(&gaslit.LexicalScorer{}))
// Final pass with the cheap NLI model:
engine.Rescore(ctx, loaded) // uses the engine's configured scorer
```

`ReclassifySummary` is pure (no model calls) — perfect for tuning thresholds or
quadrant tallying. `Rescore` re-scores each diagnostic's stored arguments
(`r1`/`r2`) and reclassifies, so you only pay for the cheap NLI check, never a
fresh debate.

## 9. Common patterns

**Detect a hallucinating agent (high confidence + contradictory facts).**

```go
cfg.Thresholds.Confidence = 0.55
cfg.Thresholds.Consistency = 0.60
cfg.GrillStrategy = gaslit.GrillAllDrifted
// check summary.Diagnostics[name].Quadrant == gaslit.QuadrantHallucinating
```

**Audit many claims cheaply with a deterministic scorer.**

```go
cfg.ConsistencyScorer = &gaslit.LexicalScorer{}
```

**Prevent an agent from being grilled.**

```go
cfg.AgentOverrides["true_historian"] = gaslit.AgentConfig{
	SkipPhases: map[gaslit.Phase]bool{gaslit.PhaseGrilling: true},
}
```

**Strict evaluation for high-stakes questions.**

```go
cfg := gaslit.PresetStrict()
```

## 10. Tips

- **Verbose logging** (`WithVerbose(true)`) prints a per-phase progress trail
  through `slog` — invaluable when debugging why an agent drifted.
- **Rate limits:** LLM APIs throttle under parallel load; set `PacingDelay` and
  `Concurrency` to taste, or wrap the transport with backoff (see
  `examples/basic`).
- **Budget errors** surface as `*gaslit.BudgetError`; check with
  `errors.As(err, &berr)`.
- **Open-ended + `LexicalScorer`** gives you a fully deterministic, zero-extra-
  call evaluation for smoke tests.
