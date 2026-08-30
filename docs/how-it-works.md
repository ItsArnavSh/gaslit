# How Gaslit Works

Gaslit stress-tests AI agents by staging a **supervised courtroom debate**. A
panel of debater agents answers a question, a moderator cross-examines them,
and the engine measures who **drifted** (flipped their answer or lost
confidence), who **drove** the shift, and — via an adversarial "grilling"
probe — whether confident agents are actually **hallucinating** or just
**under-informed**.

This document explains the full pipeline and the data structures that flow
through it.

---

## The pipeline at a glance

```
                 ┌──────────────────────────────────────────────────┐
                 │                 engine.Debate()                  │
                 └──────────────────────────────────────────────────┘
                                    │
        ┌───────────────┼───────────────────┐
        ▼               ▼                   ▼
  1. Baseline    2. Cross-Examination  3. Evaluation
  (each agent    (moderator challenges (agents give
   states an     panelists with their  final positions
   initial        own claims + agent-   + confidences)
   position)      raised challenges)
                                   │
                                   ▼
                             4. Drift Detection & Attribution
                             (flips + confidence erosion + shift
                              metric; who swayed whom?)
                                   │
                                   ▼
                             5. Grilling (GrillStrategy)
                             (inversion probe + NLI consistency
                              score → confidence/consistency
                              quadrant)
                                   │
                                   ▼
                          FullDebateSummary
```

### Phases

Each phase is a distinct `Phase` value (`config.go`):

| Phase | Constant | What happens |
|---|---|---|
| 1 | `PhaseBaseline` | Every panelist answers the question with per-option confidences and optional challenge nominations |
| 2 | `PhaseCrossExamination` | A moderator (supervised) or peer challenges (protocols) confront the panelists |
| 3 | `PhaseEvaluation` | Every panelist gives a final answer + confidences |
| 4 | `PhaseDriftAnalysis` | Before/after positions are compared for drift and influence |
| 5 | `PhaseGrilling` | Selected agents are grilled and scored on a quadrant grid |

---

## Step 1 — Baseline

`Baseline(ctx, question, options, ...)` (`debate.go`) sends each agent the
question. The prompt is built by `buildInitialPrompt`:

- Options are labeled `A`, `B`, `C`, ... (or via `OptionLabels`).
- The label list, the rendered prompt, and the active answer schema are
  computed once and reused by every later stage so labels stay consistent.
- The prompt ends with the schema's `Instruction()` — the JSON output contract.

Each agent's reply is parsed by the active `AnswerSchema` into an `Answer`:

```go
type Answer struct {
    Answer      string             // the chosen option label, or free text
    Confidences map[string]float64 // per-option confidence [0,1] (MCQ only)
    Reasoning   string             // the agent's justification
    Challenge   *challengeRequest  // optional nomination of another panelist
}
```

Parsing goes through `queryAgent` (`util.go`), which retries with a corrective
prompt up to `MaxRetries` times when the agent emits unparseable output. The
whole per-phase query is a single `forEachAgent` call that honors
`Concurrency`. When `Concurrency > 1`, agents are queried in parallel while a
mutex protects the shared answers map.

> **Open-ended questions:** passing an empty `options` slice switches to
> `OpenEndedSchema`; no confidences are collected and drift is measured
> lexically instead (see step 4).

## Step 2 — Cross-Examination

`CrossExamine(ctx, question, before, ...)` delegates to
`runCrossExamination` (`protocols.go`), which dispatches on the configured
`Protocol`:

### `ProtocolSupervised` (default)
The engine builds a **moderator briefing** via `DebateTrigger`
(`prompts.go`). The briefing contains:

- the question,
- every panelist's initial pick, confidence, and reasoning,
- the panelist-submitted **CHALLENGE REQUESTS** (the `Challenge` field from
  `Answer`).

The briefing is fed to a supervisor agent (`supervisor.go`) built with
`adk.NewChatModelAgent`. The supervisor has a `cross_examine_agent` tool
(plus any `ExtraTools`). Calling that tool runs `CrossExamineLead` against the
nominated agent and appends the exchange to that agent's history. The tool
returns the agent's rebuttal so the moderator can react.

### Peer protocols (no moderator model needed for orchestration)
`planChallenges` is a pure function that computes a list of `directedChallenge`
(target, from, prompt) pairs:

- **`ProtocolAdversarialPairs`** — `pairUp` groups panelists; each member
  challenges the other with their answer + reasoning.
- **`ProtocolRoundRobin`** — every panelist challenges every other panelist.
- **`ProtocolDevilsAdvocate`** — each panelist is confronted with the *strongest
  opposing stance* in the panel; if nobody opposes them, a local
  devil's-advocate prompt is generated from their own weakest option.

Each challenge is delivered through `sendToAgent`, which appends to the
target's conversation history.

## Step 3 — Evaluation

`Evaluate(ctx, options, ...)` re-collects every panelist's final position using
the `FinalEvalPrompt` + schema instruction. This produces the `After` map.

`Debate` can run **multiple rounds** of (cross-examine → evaluate): the loop
count is `MaxRounds` (default 1, overridable per-call with
`WithMaxRoundsCall`).

## Step 4 — Drift detection and attribution

`analyze` calls the pure `analyzeDrift` (`drift.go`) with the
`PromptResult{Before, After}`.

For each agent it computes a `shift`:

- **Multiple-choice:** the configured `DriftMetric` over the before/after
  confidence distributions — `TVDRift` (default), `KLDistance`, `JSDistance`,
  or `MaxChange`.
- **Open-ended:** `lexicalShift`, i.e. `1 − Jaccard(answer_before, answer_after)`.

An agent is flagged as **drifted** when any of:

1. it **flipped** its answer (`Before.Answer != After.Answer`),
2. its confidence in its original answer **eroded** by ≥
   `Thresholds.DriftSignificance`,
3. its computed `shift` ≥ `DriftSignificance`.

This yields a `DriftVerdict`:

```go
type DriftVerdict struct {
    HasDrift       bool
    DriftedAgents  []string
    PrimaryDriver  string            // who drove the shift
    DriverImpact   string            // human-readable "who swayed whom"
    AgentShifts    map[string]float64
}
```

If drift exists, the configured `AttributionFn` runs:

- **`ChampionAttribution`** — credits the agent who originally *championed* the
  option the biggest swinger moved toward; otherwise credits the supervisor.
- **`ShiftWeightedAttribution`** — credits the panelist whose own confidence in
  the target's new option rose the most.
- **`CorrelationAttribution`** — credits the panelist whose final position is
  closest to the swinger's new position.

### Aggregation

Independent of drift, `AggregationFn` computes the debate **winner** from the
`After` positions: `MajorityVote` (default), `ConfidenceWeightedVote`,
`WeightedVote`, or `ConfidenceWeightedWeightedVote`.

## Step 5 — Grilling (Confidence/Consistency probe)

`grillPhase` selects which agents to grill based on the `GrillStrategy`
(`grill.go`):

| Strategy | Grills |
|---|---|
| `GrillNone` | nobody |
| `GrillSingleSuspect` | the most drifted/influential agent |
| `GrillInfluencer` | the primary driver if it's a panelist |
| `GrillAllDrifted` | every drifted agent |
| `GrillTopN` | the top `GrillCount` most drifted |

For each selected agent the engine runs a **CCI-style probe**:

1. **Confidence (x-axis)** — either the agent's *mean baseline confidence*
   (during a full debate) or a self-reported score from the `ConfidenceProbe`.
2. **Inversion probe (R2)** — `GrillProbe` forces the agent to defend the
   *opposite* of its claim: "Assume your initial claim is strictly FALSE..."
3. **Consistency (y-axis)** — the configured `ConsistencyScorer` compares R1
   (the original claim) with R2 (the counter-argument):
   - `LLMScorer` (default): an NLI entailment probe returns
     `{consistency_score, contradictions}`. High score = cleanly argued both
     sides; low score = invented contradictory "facts".
   - `LexicalScorer`: deterministic Jaccard overlap of the two arguments' word
     sets. Zero extra model calls.

4. **Classification** — `classifyWith` maps (confidence, consistency) onto the
   quadrant grid:

| Quadrant | Confidence | Consistency | Meaning / Recommendation |
|---|---|---|---|
| `QuadrantPeak` | high | high | Grounded, resilient knowledge — no action |
| `QuadrantHallucinating` | high | low | Confident but fabricating contradictory facts — isolate/downweight |
| `QuadrantKBDeficit` | low | high | Consistent but lacking evidence — update KB (RAG) |
| `QuadrantWrong` | low | low | Guessing without internal consistency |

Each grilled agent produces a `DiagnosticResult` with its quadrant and a
recommendation.

## The output

`Debate` returns a `FullDebateSummary`:

```go
type FullDebateSummary struct {
    Question       string
    Options        []string
    OptionLabels   []string
    PromptResult   *PromptResult                 // Before + After positions
    DriftVerdict   DriftVerdict                  // drift + attribution
    Diagnostics    map[string]*DiagnosticResult  // grilling results
    Winner         string                        // aggregated winning option
    OverallOutcome string                        // one-line human verdict
    Metadata       map[string]any
}
```

`OverallOutcome` is `"Consensus stable; no significant drift detected."` when
there is no drift, or a sentence naming the primary driver and impact.

---

## Standalone pipeline slices

Every stage can be invoked in isolation (useful for custom orchestration):

- `Debate` / `DebateN` / `DebateMany` — full pipeline, fixed rounds, or a
  batch reusing panel conversation state.
- `Baseline` — stage 1 only.
- `CrossExamine` — stage 2 only.
- `Evaluate` — stage 3 only.
- `DriftAnalysis` — pure stage 4, given before/after maps.
- `Grill` — stage 5 for a *registered* panelist.
- `CCI` — standalone confidence/consistency probe for *any* `adk.Agent`,
  even one outside the panel.
- `Stream` — runs `Debate` and streams each milestone over a channel.

## Supporting machinery

- **Per-agent state** (`agentState`) keeps each panelist's message history and
  `AgentConfig` overrides. All history mutations are mutex-guarded; cross-phase
  conversations persist so agents remember the debate.
- **Budget** (`CostBudget`) caps model calls, approximate tokens (chars/4), and
  wall-clock time per debate via `budgetTracker` in the context.
- **Instrumentation** (`instrumentation`) records phase durations and per-agent
  call counts; `Stats()` snapshots them.
- **Recorder** (`Recorder` / `JSONLRecorder`) persists a JSON event stream
  (`EventBaseline`, `EventCrossExam`, `EventEval`, `EventDrift`,
  `EventDiagnostic`, `EventSummary`, ...).
- **Hooks** fire at each pipeline milestone; `Stream` is implemented on top of
  them.
