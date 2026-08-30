# File Structure & Reference

The module is organized into a public facade and feature packages:

- The **module root** (`package gaslit`) is the public API. It is a thin facade:
  every exported type is an *alias* and every exported function/variable is a
  *re-export* over the internal packages, so the surface is identical to a
  single flat package while the code stays organized by feature.
- **`internal/`** holds the implementation. Each folder is its own package;
  `core` is the shared leaf (types + pure logic), and the feature packages
  (`drift`, `aggregation`, `scoring`, `persist`, `engine`) build on it.

```
.
├── types.go          Public facade: type aliases over internal packages
├── consts.go         Public facade: re-exported constants (enums, quadrants, events, schema text)
├── funcs.go          Public facade: re-exported functions and variables (New, presets, options, ...)
├── facade_test.go    End-to-end smoke test of the public API
├── internal/
│   ├── core/         Shared types and pure logic (leaf; imported by every other package)
│   │   ├── types.go       Phase, Thresholds, CostBudget, AgentConfig, Protocol, GrillStrategy,
│   │   │                  QuadrantState, DiagnosticResult, Answer, PromptResult,
│   │   │                  FullDebateSummary, DriftVerdict, Events, Recorder, BudgetError,
│   │   │                  InstrumentationStats, StreamEvent
│   │   ├── config.go      Config struct
│   │   ├── options.go     Option/CallOption types, presets, With* / With*Call constructors
│   │   ├── functypes.go   DriftMetric/AttributionFn/AggregationFn/Classifier/ConsistencyScorer
│   │   │                  types; ThresholdClassifier, ClassifyWith, ReclassifySummary, helpers
│   │   ├── schema.go      AnswerSchema (MCQ / OpenEnded), JSON extraction/parsing
│   │   ├── prompts.go     PromptTemplates and the built-in templates
│   │   └── hooks.go       Hooks + MergeHooks
│   ├── drift/        Drift metrics, attribution functions, analyzeDrift
│   ├── aggregation/  Winning-option aggregation functions
│   ├── scoring/      ConsistencyScorer implementations (LLMScorer, LexicalScorer)
│   ├── persist/      Event recording, JSONL recorder, summary save/load
│   └── engine/       Gaslit engine: construction, debate pipeline, grilling, CCI,
│                     protocols, agent I/O, budget accounting, instrumentation,
│                     streaming, Rescore
└── examples/
    └── basic/        Runnable Groq demo (main.go, sample_agents.go)
```

---

## Root facade

The root package re-exports the whole public surface:

- **`types.go`** — `type Config = core.Config`, `type Gaslit = engine.Gaslit`,
  `type LLMScorer = scoring.LLMScorer`, `type JSONLRecorder = persist.JSONLRecorder`,
  and every other public type.
- **`consts.go`** — re-declares all enum values, quadrant/event constants, and
  the schema contract strings.
- **`funcs.go`** — `New` (wraps `engine.New`) plus re-exported `var`s for the
  presets, all `With*`/`With*Call` options, drift metrics, attribution,
  aggregation, persistence, and `ReclassifySummary`.
- **`facade_test.go`** — an end-to-end test through the public API only, so the
  facade can never silently drift from the documented surface.

Because `Gaslit` is an alias, every engine method (`Debate`, `Grill`, `CCI`,
`Rescore`, `Stream`, `Stats`, ...) is callable through `*gaslit.Gaslit` without
wrappers.

## internal/core

The leaf package: shared data types and pure logic. It imports nothing internal.

- **`types.go`** — `Phase`, `Thresholds` (+ `Resolve`), `CostBudget`,
  `AgentConfig`, `Protocol` (+ `String`), `GrillStrategy` (+ `String`),
  `QuadrantState` + `QuadrantPeak/Hallucinating/KBDeficit/Wrong`,
  `DiagnosticResult`, `Answer`, `PromptResult`, `FullDebateSummary`,
  `DriftVerdict`, `EventType` + `Event*` consts, `Event`, `Recorder`,
  `BudgetError`, `InstrumentationStats`, `StreamEvent`.
- **`config.go`** — the `Config` struct (fields documented in
  [usage-guide.md](./usage-guide.md)).
- **`options.go`** — `Option`, `CallOption`, `CallOptions`; presets
  `PresetDefault`/`PresetStrict`/`PresetLenient`; all pure field-setting
  `With*`/`With*Call` constructors. (`WithLexicalScorer`/`WithLLMScorer` live in
  `engine` because they construct scorer implementations.)
- **`functypes.go`** — the pluggable function-type definitions (`DriftMetric`,
  `AttributionFn`, `AggregationFn`, `Classifier`, `ConsistencyScorer`);
  `ThresholdClassifier`, `ClassifyWith`, `ReclassifySummary`; helpers
  `Clamp01`, `Tokenize`, `ExtractJSONObject`.
- **`schema.go`** — `AnswerSchema`, `MCQSchema`, `OpenEndedSchema`,
  `DefaultJSONInstruction`, `OpenEndedInstruction`.
- **`prompts.go`** — `PromptTemplates` (+ `Resolve`) and the built-in templates,
  including `DefaultNLIProbe` (used by `scoring.LLMScorer`'s fallback).
- **`hooks.go`** — `Hooks` and `MergeHooks`.

## internal/drift

The drift-detection math and influence attribution (pure functions).

- `TVDRift`, `KLDistance`, `JSDistance`, `MaxChange` — the built-in `DriftMetric`s.
- `ChampionAttribution`, `ShiftWeightedAttribution`, `CorrelationAttribution` —
  the built-in `AttributionFn`s.
- **`Analyze(res, labels, thr, metric, attribution) DriftVerdict`** — flags
  drifted agents (flip / erosion / shift) and runs attribution. For open-ended
  questions it falls back to `lexicalShift` (1 − Jaccard).

## internal/aggregation

Winning-option strategies: `MajorityVote`, `ConfidenceWeightedVote`,
`WeightedVote`, `ConfidenceWeightedWeightedVote` (+ `topOption` helpers).

## internal/scoring

`ConsistencyScorer` implementations:

- **`LLMScorer`** — NLI entailment probe using a `model.BaseModel`; parses
  `{consistency_score, contradictions}`; falls back to 0.5 on parse failure.
  Falls back to `core.DefaultNLIProbe` when no `NLIProbe` is wired.
- **`LexicalScorer`** — deterministic Jaccard similarity of word sets; no model
  calls. A weak proxy, useful while debugging the pipeline.

## internal/persist

- `JSONLRecorder` / `NewJSONLRecorder` — one `core.Event` per line.
- `SaveSummary` / `LoadSummary` — pretty-print / read a `FullDebateSummary`.
- `SaveSummaryFile` / `LoadSummaryFile` — disk variants for caching a debate run.

## internal/engine

The `Gaslit` orchestration layer (the bulk of the pipeline).

- **`engine.go`** — `Gaslit` struct, `New` (applies options, resolves defaults,
  builds the supervisor), `resolveConfig`, budget accounting
  (`checkBudget`/`accountCall`/`withBudgetContext`), `redact`, `pace`, `record`,
  and the `WithLexicalScorer`/`WithLLMScorer` options.
- **`agent_handler.go`** — `consumeAgentOutput`: drains an eino agent stream into
  a message.
- **`options.go`** — `resolveCall`, `hooks`, `logf` (per-call merging).
- **`debate.go`** — `Debate`, `DebateN`, `Baseline`, `CrossExamine`, `Evaluate`,
  `DriftAnalysis`, `analyze`, `buildInitialPrompt`, phase/skip helpers.
- **`grill.go`** — `CCI`, `Grill`, `selfConfidence`, `inversionProbe`, `score`,
  `grillPhase`, `grillTargets`, `topDrifted`.
- **`protocols.go`** — `runCrossExamination`, `planChallenges`, `pairUp`,
  `devilsAdvocateLocal`.
- **`schema.go`** — `schemaFor` (picks open-ended for empty options).
- **`supervisor.go`** — `createCrossExamineTool`, `createSupervisorAgent`,
  `runSupervisor`.
- **`util.go`** — `queryAgent`, `sendToAgent`, `runAgent`, `forEachAgent`.
- **`instrument.go`** — phase durations and per-agent call counts; `Stats()`.
- **`batch.go`** — `DebateMany`.
- **`stream.go`** — `Stream` (event streaming via injected hooks).
- **`rescore.go`** — `Rescore`: re-run only the consistency scorer over a saved
  summary's stored R1/R2, then reclassify. No new baseline/debate calls.

## Examples

```
examples/
└── basic/
    ├── main.go            Full runnable demo (Groq API): model split, budget, cache/rescore
    └── sample_agents.go   History "panel of historians" + 429-backoff transport
```

Run with:

```sh
GROQ_API_KEY=... go run ./examples/basic          # debug: LexicalScorer, no NLI calls
GROQ_API_KEY=... go run ./examples/basic -final   # benchmark: cheap LLM NLI scorer
```
