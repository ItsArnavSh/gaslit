# gaslit

A Go framework that stress-tests AI agents by staging a **supervised courtroom debate**. Gaslit runs a panel of debater agents against a multiple-choice question, cross-examines them with a moderator, and detects when agents **drift** or **hallucinate**.

Built on top of [cloudwego/eino](https://github.com/cloudwego/eino).

## How it works

1. **Baseline** — every agent answers the question with per-option confidences.
2. **Cross-examination** — a supervisor moderator challenges panelists using their own claims and any challenges agents raised, via a `cross_examine_agent` tool.
3. **Re-evaluation** — agents give final positions.
4. **Drift detection** — flips, confidence erosion, and total variation distance are measured, and influence is attributed to the swaying agent (or the supervisor).
5. **Grill** — the single most drifted/influential agent is grilled with an inversion probe and scored on a confidence/consistency quadrant.

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

	// 2. Initialize the engine with a supervisor model + the panel.
	engine, err := gaslit.Init(ctx, myModel, []adk.Agent{econ, pol})
	if err != nil {
		log.Fatal(err)
	}

	// 3. Run the debate.
	summary, err := engine.Debate(ctx, "What caused the Roman Empire's collapse?", []string{
		"Economic exhaustion",
		"Military breakdown",
	}, true)
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

## API

- `Init(ctx, supervisorModel, agents) (*Gaslit, error)` — build an engine from a panel of `adk.Agent`s and a supervisor model.
- `(*Gaslit).Debate(ctx, question, options, verbose) (*FullDebateSummary, error)` — run the full baseline → cross-examine → evaluate → drift → grill pipeline.
- `FullDebateSummary`, `PromptResult`, `DriftVerdict`, `DiagnosticResult`, `QuadrantState` — result types describing positions, drift, and per-agent diagnostics.

## Example

See [`examples/basic`](./examples/basic) for a full runnable program using Groq's API, including a 429-backoff transport and a "panel of historians" demo.

```sh
GROQ_API_KEY=... go run ./examples/basic
```

## License

[MIT](./LICENSE)
