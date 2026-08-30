# File Structure & Reference

All source files live in the module root (package `gaslit`). This document
lists each file, what it owns, and the important functions/types it defines.

```
.
├── api.go            Engine construction (New)
├── agent_handler.go  Raw agent event-stream consumption
├── aggregation.go    Winning-option aggregation functions
├── batch.go          DebateMany batching
├── cci.go            Confidence/Consistency probes, quadrants, classifiers, scorers
├── config.go         Config, presets, Options, Phase/Protocol/GrillStrategy enums
├── debate.go         Pipeline stages: Baseline, CrossExamine, Evaluate, Debate
├── drift.go          Drift metrics, attribution functions, analyzeDrift
├── entity.go         Core structs (Gaslit, Answer, PromptResult, summaries), budget
├── grill.go          Grilling diagnostics, CCI, inversion probe, target selection
├── hooks.go          Observability callbacks
├── instrument.go     Phase-duration and per-agent-call instrumentation
├── options.go        Per-call CallOption configuration
├── persist.go        Event recording, JSONL recorder, summary save/load
├── prompts.go        Default prompt templates and the PromptTemplates override type
├── protocols.go      Cross-examination orchestration strategies
├── schema.go         AnswerSchema (MCQ / OpenEnded), JSON extraction/parsing
├── stream.go         Streaming pipeline events
├── supervisor.go     Moderator agent + cross_examine_agent tool
└── util.go           Agent querying, history management, concurrency helpers
```

---

## api.go

Entry point for building an engine.

- **`New(ctx, cfg Config, opts ...Option) (*Gaslit, error)`**
  Applies `Option`s, resolves the config, validates that a supervisor model and
  at least one agent are present and that agent names are unique, wires the LLM
  scorer to the NLI template, builds the `cross_examine_agent` tool and the
  supervisor agent, and returns a ready-to-use `*Gaslit`.

## agent_handler.go

Bridge between eino's async agent iterator and Gaslit's message model.

- **`consumeAgentOutput(iter *adk.AsyncIterator[*adk.AgentEvent]) (*schema.Message, error)`**
  Drains an agent's event stream, keeps the latest non-empty assistant message
  (or concatenates streamed chunks), and returns it as a `*schema.Message`.
  Errors in the stream are propagated.

## aggregation.go

Turn the panel's *final* positions into a single winning option.

- `type AggregationFn func(after map[string]*Answer, labels []string, weights map[string]float64) string`
- **`MajorityVote`** — most votes; requires a strict majority (`> n/2`).
- **`ConfidenceWeightedVote`** — votes weighted by each agent's confidence in
  its own pick.
- **`WeightedVote`** — votes weighted by the per-agent `AgentConfig.Weight`
  (default 1.0).
- **`ConfidenceWeightedWeightedVote`** — agent weight × confidence.
- `topOption`, `topOptionFloat` — internal helpers; return `""` when no winner.

## batch.go

- **`(*Gaslit) DebateMany(ctx, questions, optionsList, opts...) ([]*FullDebateSummary, error)`**
  Runs `Debate` for each (question, options) pair in order, **reusing panel
  conversation state** so agents carry context across questions. Errors abort
  the batch.

## cci.go

The confidence/consistency (CCI) core and quadrant taxonomy.

- **`type QuadrantState string`** — `QuadrantPeak`,
  `QuadrantHallucinating`, `QuadrantKBDeficit`, `QuadrantWrong`.
- **`type DiagnosticResult struct`** — agent name, confidence (x), consistency
  (y), quadrant, R1 (initial reasoning), R2 (counter reasoning), recommendation.
- `type Classifier func(conf, consistency float64) (QuadrantState, string)`
- **`ThresholdClassifier`** — default classifier (delegates to `classifyWith`).
- **`classifyWith(conf, consistency float64, thr Thresholds)`** — the actual
  quadrant decision table.
- `type ConsistencyScorer interface { Score(ctx, question, r1, r2) (float64, error) }`
- **`LLMScorer`** — NLI entailment probe using a model; parses
  `{consistency_score, contradictions}`; falls back to 0.5 on parse failure.
- **`LexicalScorer`** — deterministic Jaccard similarity of word sets; no model
  calls.

## config.go

Construction-time configuration.

- **`type Phase int`** — `PhaseBaseline`, `PhaseCrossExamination`,
  `PhaseEvaluation`, `PhaseDriftAnalysis`, `PhaseGrilling` (+ `String()`).
- **`type Thresholds struct`** — `DriftSignificance`, `Confidence`, `Consistency`
  (+ `resolve()` for defaults).
- **`type CostBudget struct`** — `MaxCalls`, `MaxTokens`, `Deadline`.
- **`type AgentConfig struct`** — `Instruction`, `Weight`, `SkipPhases`.
- **`type Config struct`** — the full engine configuration (see
  [usage-guide.md](./usage-guide.md) for a field-by-field tour).
- **`type Option func(*Config)`**
- **`PresetDefault()` / `PresetStrict()` / `PresetLenient()`**
- Option constructors: `WithLogger`, `WithPacingDelay`, `WithRequestTimeout`,
  `WithMaxRetries`, `WithConcurrency`, `WithMaxRounds`, `WithThresholds`,
  `WithOptionLabels`, `WithAnswerSchema`, `WithProtocol`, `WithGrillStrategy`,
  `WithGrillCount`, `WithAggregation`, `WithDriftMetric`, `WithAttribution`,
  `WithClassifier`, `WithConsistencyScorer`, `WithPromptTemplates`,
  `WithAgentConfig`, `WithExtraTools`, `WithHooks`, `WithRecorder`,
  `WithRedact`, `WithBudget`, `WithVerbose`.
- **`type Protocol int`** — `ProtocolSupervised`, `ProtocolAdversarialPairs`,
  `ProtocolRoundRobin`, `ProtocolDevilsAdvocate`.
- **`type GrillStrategy int`** — `GrillNone`, `GrillSingleSuspect`,
  `GrillInfluencer`, `GrillAllDrifted`, `GrillTopN`.
- **`(Config) resolve() (Config, error)`** — fills defaults and validates
  required fields (supervisor model, ≥1 agent).

## debate.go

The orchestrator. Methods on `*Gaslit`:

- **`Debate(ctx, question, options, opts...)`** — full pipeline; returns
  `*FullDebateSummary`.
- **`DebateN(ctx, question, options, rounds, opts...)`** — `Debate` with a
  specific round count.
- **`Baseline(ctx, question, options, opts...)`** — stage 1; returns the
  `Before` map (`map[string]*Answer`).
- **`CrossExamine(ctx, question, before, opts...)`** — stage 2.
- **`Evaluate(ctx, options, opts...)`** — stage 3; returns the `After` map.
- **`DriftAnalysis(before, after, options, opts...)`** — pure stage 4,
  returns a `DriftVerdict`.
- **`analyze(ctx, res, labels, co)`** — wires hooks/instrumentation/recording
  around `analyzeDrift`.
- **`buildInitialPrompt(question, options, co)`** — computes labels, prompt,
  and schema; reused by every stage.
- `autoLabels(n)` — generates `A`, `B`, `C`, ...
- `stateSkipped`, `phaseStart`, `phaseEnd` — helpers for phase skipping and
  hook/instrumentation plumbing.

## drift.go

Drift math and influence attribution (all pure functions).

- **`type DriftMetric func(labels, before, after) float64`**
- **`TVDRift`** — total variation distance (`Σ|Δ| / 2`).
- **`KLDistance`** — KL divergence (ε-smoothed).
- **`JSDistance`** — Jensen–Shannon divergence (symmetric, bounded).
- **`MaxChange`** — max absolute per-option change.
- **`type AttributionFn func(res *PromptResult, labels, verdict)`**
- **`ChampionAttribution`** — credits the champion of the target's new option,
  else the supervisor.
- **`ShiftWeightedAttribution`** — credits the panelist whose confidence in the
  new option rose most.
- **`CorrelationAttribution`** — credits the panelist whose final position is
  closest to the swinger's.
- **`type DriftVerdict struct`** — `HasDrift`, `DriftedAgents`, `PrimaryDriver`,
  `DriverImpact`, `AgentShifts`.
- **`analyzeDrift(res, labels, thr, metric, attribution)`** — flags drifted
  agents (flip / erosion / shift) and runs attribution.
- `lexicalShift` — `1 − Jaccard` for open-ended answers.
- `tokenize`, `clamp01`, `biggestShiftAgent` — helpers.

## entity.go

Core data structures and engine state.

- **`type Answer struct`** — `Answer`, `Confidences`, `Reasoning`, `Challenge`.
- `type agentState struct` — agent, message history, `AgentConfig`, mutex.
- **`type Gaslit struct`** — config, supervisor, per-agent states, logger,
  instrumentation, atomic call/token counters (budget accounting).
- **`type PromptResult struct`** — `Before`/`After` answer maps.
- **`type FullDebateSummary struct`** — question/options/labels, positions,
  verdict, diagnostics, winner, outcome, metadata.
- `redact(s)` — applies the configured redaction function.
- `pace(ctx)` — sleeps `PacingDelay` between calls (honors ctx cancellation).
- `checkBudget(ctx)` / `accountCall(agent, content)` — budget accounting.
- `withBudgetContext(ctx, budget)` — attaches a debate-scoped budget tracker and
  the optional deadline.
- **`type BudgetError struct`** — returned when a budget is exhausted.

## grill.go

The grilling / CCI execution layer.

- **`CCI(ctx, agent, statement, question, opts...)`** — standalone
  confidence/consistency probe against **any** `adk.Agent`. Confidence comes
  from the self-report probe unless overridden with `WithConfidenceCall`.
- **`Grill(ctx, agentName, claim, question, opts...)`** — same probe for a
  **registered** panelist.
- **`selfConfidence(ctx, st, claim)`** — runs `ConfidenceProbe`, parses
  `{"confidence": 0..1}`.
- **`inversionProbe(ctx, st, question, claim)`** — runs `GrillProbe` (defend
  the opposite) and returns R2.
- **`score(ctx, question, r1, r2)`** — runs the configured `ConsistencyScorer`
  with budget/pacing/accounting.
- **`grillPhase(ctx, question, res, verdict, labels, co)`** — selects targets
  via the active strategy and produces `map[string]*DiagnosticResult`.
- **`grillTargets(verdict, co)`** — strategy → target list.
- `topDrifted`, `confidenceSlice`, `mean`, `parseConfidence` — helpers.

## hooks.go

- **`type Hooks struct`** — `OnPhaseStart`, `OnPhaseEnd`, `OnBaseline`,
  `OnCrossExam`, `OnEval`, `OnDrift`, `OnDiagnostic`.
- **`mergeHooks(base, override)`** — non-nil override fields win.

## instrument.go

- **`type instrumentation struct`** — phase start times, phase durations,
  per-agent call counts (mutex-guarded).
- **`(*Gaslit) Stats() InstrumentationStats`** — snapshot of total calls,
  tokens, per-agent calls, and phase durations.

## options.go

Per-call configuration.

- **`type CallOption func(*CallOptions)`**
- **`type CallOptions struct`** — resolved per-call view of verbose, rounds,
  labels, thresholds, grill strategy/count, protocol, metadata, confidence
  override, claim, hooks, budget.
- Constructors: `WithVerboseCall`, `WithMaxRoundsCall`, `WithOptionLabelsCall`,
  `WithThresholdsCall`, `WithGrillStrategyCall`, `WithGrillCountCall`,
  `WithProtocolCall`, `WithMetadataCall`, `WithConfidenceCall`,
  `WithClaimCall`, `WithHooksCall`, `WithBudgetCall`.
- **`resolveCall(opts...)`** — merges call options over engine config.
- `hooks(co)` — effective hooks for a call.
- `logf(co, ...)` — verbose logging guard.

## persist.go

- **`type EventType string`** — `EventPhaseStart`, `EventPhaseEnd`,
  `EventBaseline`, `EventCrossExam`, `EventEval`, `EventDrift`,
  `EventDiagnostic`, `EventSummary`, `EventDone`, `EventError`.
- **`type Event struct`** — timestamp, type, agent, raw JSON data.
- **`type Recorder interface { Record(ctx, Event) error }`**
- **`JSONLRecorder`** / **`NewJSONLRecorder(w)`** — one JSON event per line.
- **`(*Gaslit) record(evType, agent, data)`** — marshals and forwards to the
  recorder, if any.
- **`SaveSummary(w, s)`** / **`LoadSummary(r)`** — pretty-print / read a
  `FullDebateSummary`.

## prompts.go

- Default prompt constants: `DefaultJSONInstruction`, `OpenEndedInstruction`,
  `defaultSupervisorSystem`, `defaultCrossExamineLead`,
  `defaultFinalEvalPrompt`, `defaultGrillProbe`, `defaultNLIProbe`,
  `defaultConfidenceProbe`.
- **`type PromptTemplates struct`** — overridable prompt builders:
  `SupervisorSystem`, `CrossExamineLead`, `FinalEvalPrompt`, `InitialPrompt`,
  `DebateTrigger`, `GrillProbe`, `NLIProbe`, `ChallengePrompt`,
  `ConfidenceProbe`.
- Default builders and **`(PromptTemplates) resolve()`** (fills zero fields).

## protocols.go

- `type directedChallenge struct` — target, from, prompt.
- **`runCrossExamination(ctx, question, before, co)`** — dispatches supervised
  vs. peer protocols.
- **`planChallenges(p, names, before, challengePrompt)`** — pure challenge
  planning for the peer protocols.
- `pairUp(names)` — grouping helper for adversarial pairs.
- `devilsAdvocateLocal(ans)` — fallback challenge from the agent's own weakest
  option.

## schema.go

- **`type AnswerSchema interface`** — `Instruction()`, `Parse(raw)`.
- **`MCQSchema`** — default; requires per-option confidences.
- **`OpenEndedSchema`** — free-form answers.
- **`(*Gaslit) schemaFor(options)`** — picks open-ended for empty options.
- **`extractJSONObject(raw)`** — finds the outermost balanced JSON object
  (tolerates leading prose / markdown fences).
- **`unmarshalAnswer(raw, requireConfidences)`** — extracts and decodes an
  `Answer`, validating non-empty answer/confidences.

## stream.go

- **`type StreamEvent struct`** — type, phase, agent, prompt, answer, verdict,
  diagnostic, metadata, error.
- **`(*Gaslit) Stream(ctx, question, options, opts...) (<-chan StreamEvent, error)`**
  Runs `Debate` in a goroutine, forwarding every milestone via a buffered
  channel (implemented with an injected `Hooks` set), and closes the channel
  when done. The caller must drain it.

## supervisor.go

- **`createCrossExamineTool()`** — builds the `cross_examine_agent` tool via
  `utils.InferTool`; the handler looks up the target agent, runs
  `CrossExamineLead` against it, and returns its rebuttal.
- **`createSupervisorAgent(ctx, model, tools)`** — builds the moderator
  `adk.ChatModelAgent` with the supervisor system prompt and tools.
- **`runSupervisor(ctx, prompt)`** — runs the moderator with pacing, budget
  accounting, redaction, and per-request timeout.

## util.go

- **`queryAgent(ctx, name, prompt, resetHistory, as)`** — sends a prompt to an
  agent and parses it through the schema, re-prompting up to `MaxRetries` times.
- **`sendToAgent(ctx, st, prompt)`** — free-form send (cross-exam / grilling);
  no JSON parsing.
- **`runAgent(ctx, st, msgs)`** — one model invocation with budget/pacing/
  timeout.
- **`forEachAgent(ctx, names, fn)`** — sequential or semaphore-limited parallel
  execution honoring `Concurrency`.
- `sanitizeHistoryMessage`, `agentInstruction`, `retryHint` — helpers.

## Examples

```
examples/
└── basic/
    ├── main.go            Full runnable demo (Groq API)
    └── sample_agents.go   History "panel of historians" + 429-backoff transport
```

Run with:

```sh
GROQ_API_KEY=... go run ./examples/basic
```
