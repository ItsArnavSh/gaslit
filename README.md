# gaslit

A Go framework that stress-tests AI agents by staging a **supervised courtroom debate**. Gaslit runs a panel of debater agents against a multiple-choice (or open-ended) question, cross-examines them with a moderator, and detects when agents **drift** or **hallucinate**.

Built on top of [cloudwego/eino](https://github.com/cloudwego/eino).

## How it works

1. **Baseline** — every agent answers the question with per-option confidences.
2. **Cross-examination** — a supervisor moderator challenges panelists using their own claims and any challenges agents raised, via a `cross_examine_agent` tool.
3. **Re-evaluation** — agents give final positions.
4. **Drift detection** — flips, confidence erosion, and a pluggable shift metric are measured, and influence is attributed to the swaying agent (or the supervisor).
5. **Grill / CCI** — drifted/influential agents are grilled with an inversion probe and scored on a confidence/consistency quadrant.

### Quadrant diagnostics

| Quadrant | Confidence | Consistency | Meaning |
|---|---|---|---|
| PEAK | high | high | Grounded, resilient knowledge |
| HALLUCINATING / MALICIOUS | high | low | Confident but fabricating contradictory facts |
| KB_DEFICIT | low | high | Logically consistent but lacking evidence |
| WRONG / UNCERTAIN | low | low | Guessing without internal consistency |

## Install

```sh
go get github.com/ItsArnavSh/gaslit
```

Requires Go 1.26+.

## Usage

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

	// 1. Build your debater agents (any eino adk.Agent).
	econ, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "economic_historian",
		Description: "Analyzes events from a socio-economic lens.",
		Instruction: "You are an economic historian. Argue that economic forces drive history.",
		Model:       myModel,
	})
	pol, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "political_historian",
		Description: "Analyzes events through leadership and military strategy.",
		Instruction: "You are a political and military historian.",
		Model:       myModel,
	})

	// 2. Configure the engine: thresholds, drift metric, attribution, grill
	//    strategy, protocols, prompts, budgets, hooks, persistence, ...
	cfg := gaslit.PresetDefault()
	cfg.SupervisorModel = myModel
	cfg.Agents = []adk.Agent{econ, pol}
	cfg.Thresholds = gaslit.Thresholds{DriftSignificance: 0.15, Confidence: 0.55, Consistency: 0.60}
	cfg.DriftMetric = gaslit.TVDRift
	cfg.AttributionFn = gaslit.ChampionAttribution
	cfg.GrillStrategy = gaslit.GrillSingleSuspect
	cfg.Protocol = gaslit.ProtocolSupervised
	cfg.Concurrency = 4

	engine, err := gaslit.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Run the debate.
	summary, err := engine.Debate(ctx, "What caused the Roman Empire's collapse?", []string{
		"Economic exhaustion",
		"Military breakdown",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Outcome: %s\n", summary.OverallOutcome)
	fmt.Printf("Drift: %t (driver: %s)\n", summary.DriftVerdict.HasDrift, summary.DriftVerdict.PrimaryDriver)
	for name, diag := range summary.Diagnostics {
		fmt.Printf("%s → %s (conf %.2f, consistency %.2f)\n", name, diag.Quadrant, diag.ConfidenceScore, diag.ConsistencyScore)
	}
}
```

## Configuration

Everything is set on a `Config` struct passed to `New(ctx, cfg)`, and any field
can be refined with an `Option`:

- **Thresholds** — `Thresholds{DriftSignificance, Confidence, Consistency}`,
  also per-call via `WithThresholdsCall`.
- **Presets** — `PresetDefault()`, `PresetStrict()`, `PresetLenient()`.
- **Drift metrics** — `TVDRift`, `KLDistance`, `JSDistance`, `MaxChange`, or any
  custom `DriftMetric` function.
- **Attribution** — `ChampionAttribution`, `ShiftWeightedAttribution`,
  `CorrelationAttribution`, or a custom `AttributionFn`.
- **Quadrant classifier** — `ThresholdClassifier` or a custom `Classifier`.
- **Consistency scoring** — `*LLMScorer` (default, uses `Config.ConsistencyModel`,
  which falls back to `SupervisorModel`) or the deterministic `*LexicalScorer`
  (zero extra model calls), or a custom `ConsistencyScorer`. Convenience options:
  `WithLexicalScorer()` (debugging) and `WithLLMScorer(m)` / `WithConsistencyModel(m)`
  (cheap scoring model, strong moderator model).
- **Grill strategy** — `GrillNone`, `GrillSingleSuspect`, `GrillInfluencer`,
  `GrillAllDrifted`, `GrillTopN` (with `GrillCount`).
- **Protocols** — `ProtocolSupervised`, `ProtocolAdversarialPairs`,
  `ProtocolRoundRobin`, `ProtocolDevilsAdvocate`.
- **Aggregation** — `MajorityVote`, `ConfidenceWeightedVote`, `WeightedVote`,
  `ConfidenceWeightedWeightedVote`, or a custom `AggregationFn`.
- **Prompts** — override any template via `PromptTemplates` (initial prompt,
  debate trigger, grill probe, NLI probe, supervisor system, ...).
- **Answer schema** — `*MCQSchema` (default) or `*OpenEndedSchema` for free-form
  questions (pass empty options to `Debate`). Custom schemas implement
  `AnswerSchema`.
- **Per-agent** — `AgentConfig{Instruction, Weight, SkipPhases}` keyed by name.
- **Execution** — `PacingDelay`, `RequestTimeout`, `MaxRetries`, `Concurrency`,
  `MaxRounds`, `OptionLabels`.
- **Budget** — `CostBudget{MaxCalls, MaxTokens, Deadline}` caps total work.
- **Observability** — `Hooks` callbacks, `Recorder` (e.g. `JSONLRecorder`),
  `Instrumentation.Stats()`, `Redact`.
- **Moderator tools** — `ExtraTools` are added to the supervisor.

## Function variants

The pipeline is fully sliced; every stage is callable standalone:

- `Debate(ctx, question, options, opts...)` — full pipeline.
- `DebateN(ctx, question, options, rounds, opts...)` — multi-round debate.
- `DebateMany(ctx, questions, optionsList, opts...)` — batch, reusing context.
- `Baseline(ctx, question, options, opts...)` — stage 1 only.
- `CrossExamine(ctx, question, before, opts...)` — stage 2 only.
- `Evaluate(ctx, options, opts...)` — stage 3 only.
- `DriftAnalysis(before, after, options, opts...)` — pure drift verdict.
- `Grill(ctx, agentName, claim, question, opts...)` — grill a registered panelist.
- `CCI(ctx, agent, statement, question, opts...)` — **standalone** consistency /
  confidence probe against any `adk.Agent`, even one outside the panel.
- `Stream(ctx, question, options, opts...)` — stream pipeline events as they happen.
- `SaveSummary` / `SaveSummaryFile` / `LoadSummary` / `LoadSummaryFile` — cache a
  debate to disk.
- `ReclassifySummary(s, thresholds)` — pure quadrant re-tally of a saved summary
  (zero model calls).
- `Rescore(ctx, s, opts...)` — re-run only the consistency scorer over a saved
  summary's stored arguments, then reclassify. No new baseline/debate calls.

All variants accept per-call `CallOption`s (`WithVerboseCall`, `WithMaxRoundsCall`,
`WithThresholdsCall`, `WithMetadataCall`, `WithBudgetCall`, ...) that merge over
the engine config.

## Example

See [`examples/basic`](./examples/basic) for a full runnable program using Groq's API, including a 429-backoff transport and a "panel of historians" demo. It splits models (strong moderator, cheap `ConsistencyModel`), caps each debate with a `CostBudget`, and demonstrates the save → load → reclassify/rescore caching loop.

```sh
GROQ_API_KEY=... go run ./examples/basic          # debug: LexicalScorer, no NLI calls
GROQ_API_KEY=... go run ./examples/basic -final   # benchmark: cheap LLM NLI scorer
```

## License

[MIT](./LICENSE)
