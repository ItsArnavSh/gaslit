# Theory: Probabilistic Certainty and Consistency (PCC)

This document summarizes the research that motivates gaslit's diagnostics and
maps each idea in the paper to a concrete piece of the codebase.

> **Paper:** Haoran Wang, Maryam Khalid, Qiong Wu, Jian Gao, Cheng Cao.
> *Fact-Checking with Large Language Models via Probabilistic Certainty and
> Consistency*. arXiv:2601.02574 [cs.CL], Jan 2026.
> https://arxiv.org/abs/2601.02574

---

## 1. The problem

LLMs hallucinate. Automated fact-checking is the standard safeguard, but
mainstream LLM fact-checkers use a **retrieval-then-verification** pipeline
that treats retrieval as *mandatory* — even when the model could answer
correctly from its own parametric knowledge. This wastes search API calls and
can even introduce irrelevant noise.

The paper's central claim: **an LLM should adaptively decide whether to rely on
internal knowledge or to initiate retrieval, based on how confident it is in a
claim** — the way a human fact-checker recalls known facts before consulting
outside sources.

Prior work used *verbal confidence* (the model self-reports a number) as the
routing signal. Verbal confidence is unreliable: it depends heavily on model
calibration, which varies across models, and is sensitive to prompt design.

## 2. The two signals

PCC estimates factual confidence along **two complementary dimensions** —
"decisiveness" and "stability" — mirroring the intuition that a good verdict
comes with a rationale that survives counterfactual scrutiny.

### 2.1 Internal certainty τ(c)

For a claim *c*, the model produces a next-token probability distribution over
the verdict. Tokens are partitioned into two sets: **T** (those meaning
"True") and **F** (those meaning "False"). Class-level probabilities:

```
pT = Σ_{w∈T} P(w | c)        pF = Σ_{w∈F} P(w | c)
```

Internal certainty is:

```
τ(c) = 1                     if the top-two tokens map to the same verdict class
τ(c) = |pT − pF|             otherwise
```

Intuition: if the two most likely next tokens *agree* on the verdict class, the
model is decisively and locally stable; otherwise the *margin* between the
aggregated True/False probability mass quantifies how uncertain it is.
Because it comes from token log-probabilities rather than a self-report, it is
much less sensitive to prompt wording and model-specific calibration quirks.

### 2.2 Reasoning consistency γ(c)

Certainty alone misses *overconfidence*. To test whether the model's reasoning
is coherent under adversarial framing, the paper elicits **K rationales on each
side** of the claim:

- R⁺(c) — explanations generated under the assumption the claim is **true**
- R⁻(c) — explanations generated under the assumption the claim is **false**

A **Natural Language Inference (NLI)** model scores the pairwise contradiction
probability φ_contr(u, v) between cross-assumption rationale pairs. The mean
cross-assumption contradiction is:

```
φ̄_contr(c) = (1/K²) Σᵢ Σⱼ ( φ_contr(rᵢ⁺, rⱼ⁻) + φ_contr(rⱼ⁻, rᵢ⁺) ) / 2
```

and reasoning consistency is its complement:

```
γ(c) = 1 − φ̄_contr(c)          γ ∈ [0, 1]
```

**Canonical example from the paper:** *"The capital of Australia is Sydney."*
A knowledgeable model defends the true side with *"…the capital is Canberra,
not Sydney"* and the false side with *"Canberra is the official capital, and
Sydney… is not"* — both rationales reinforce the same factual core, so
consistency is high. An uncertain model hedges and produces overlapping,
logically weak rationales — low consistency.

## 3. The routing policy (four quadrants)

Thresholds α, β ∈ (0,1) partition (τ, γ) space into four regions, each with a
tailored verification strategy:

| Quadrant | Condition | Diagnosis | Strategy |
|---|---|---|---|
| **Direct answering** | τ ≥ α, γ ≥ β | Strong factual confidence | Answer from parametric knowledge; **no retrieval** |
| **Targeted search (consistency)** | τ ≥ α, γ < β | **Overconfident hallucination** | Find the most contradictory rationale pairs and generate focused queries targeting the disputed knowledge |
| **Targeted search (reflection)** | τ < α, γ ≥ β | **Knowledge gap** | Prompt the model to self-reflect on the missing information, then retrieve high-precision evidence |
| **Deep search** | τ < α, γ < β | High-risk, out-of-distribution | Iterative query → retrieve → reflect → re-verify loop |

The key design insight: **certainty decides *whether* to retrieve; consistency
decides *how***. High certainty + low consistency → narrow, contradiction-driven
search. Low certainty + high consistency → broad evidence exploration.

## 4. Empirical findings

- **Calibration:** PCC achieves dramatically lower Expected Calibration Error
  (ECE) than verbal confidence across SciFact, FeLMWk, and HoVER, and across
  GPT-4o/GPT-4o-mini, Gemini-2.5-Pro/Flash, and Mistral-7B-Instruct. Example:
  HoVER ECE drops from 39.37 (verbal) to 20.76 (PCC) with GPT-4o-mini.
- **Accuracy:** a PCC-guided fact-checker beats Factool, FactCheck-GPT, SAFE,
  and FIRE. Biggest relative gain is on **false claims** — e.g. +15.2%
  relative F1 for False on HoVER with GPT-4o (0.66 → 0.76).
- **Generalization:** consistent gains on proprietary and open-weight models,
  and on weaker models the benefit grows — with GPT-3.5-turbo the False score
  on FeLMWk jumps 0.23 → 0.66, nearly matching GPT-4.1.
- **Retrieval quality:** contradiction-driven targeted search beats generic
  deep search in the high-certainty/low-consistency region (up to +12 macro-F1),
  while deep search wins in the low-certainty/high-consistency region.

## 5. How gaslit instantiates PCC

gaslit is a *courtroom-debate* realization of the same two-dimensional
diagnostic: instead of routing to retrieval, it routes to **trust decisions**
about the agent. The mapping:

| PCC concept | gaslit implementation |
|---|---|
| Internal certainty τ(c) | **Confidence score** (`DiagnosticResult.ConfidenceScore`) — the agent's mean per-option confidence over the baseline (or a self-reported `ConfidenceProbe` score in `CCI`) |
| Reasoning consistency γ(c) | **Consistency score** (`DiagnosticResult.ConsistencyScore`) — an NLI/entailment probe comparing R1 (the claim) with R2 (an adversarial counter-argument) |
| True/false rationale pairs R⁺/R⁻ | **Inversion probe** (`GrillProbe`): "Assume your initial claim is strictly FALSE… provide the strongest defense of the alternative." R1 = claim, R2 = counter-argument |
| NLI contradiction model | **`ConsistencyScorer`**: `LLMScorer` (model-based entailment, default) or `LexicalScorer` (deterministic Jaccard overlap) |
| Thresholds α, β | **`Thresholds{Confidence, Consistency}`** |
| Four-quadrant routing policy | **`QuadrantState`** via `classifyWith` |

### The quadrant grid

| gaslit quadrant | Confidence (x) | Consistency (y) | PCC policy | gaslit recommendation |
|---|---|---|---|---|
| **PEAK (Grounded Knowledge)** | high | high | Direct answering | "No change needed. Agent demonstrates high factual resilience." |
| **HALLUCINATING / MALICIOUS** | high | low | Targeted search via contradiction signals | "Isolate or downweight agent. Displays high confidence but fabricated mutually exclusive premises." |
| **KB_DEFICIT (Incomplete Info)** | low | high | Targeted search via self-reflection | "Update knowledge base (RAG). Logic is consistent but lacks definitive evidence." |
| **WRONG / UNCERTAIN** | low | low | Deep search | "Agent is guessing with low conviction and inconsistent internal logic." |

The paper's quadrants and gaslit's quadrants are **isomorphic** — the 
threshold classifier is literally the same decision table:

```go
case conf >= thr.Confidence && consistency >= thr.Consistency: // Peak
case conf >= thr.Confidence && consistency <  thr.Consistency: // Hallucinating
case conf <  thr.Confidence && consistency >= thr.Consistency: // KB deficit
default:                                                       // Wrong/uncertain
```

### Where the signals come from in gaslit

The **inversion probe** is the direct analogue of the paper's adversarial
rationale elicitation:

```go
const defaultGrillProbe = `CRITICAL INVERSION PROBE:
Question: %q
Your baseline claim was: %q
Assume your initial claim is strictly FALSE. Provide the strongest factual
defense and historical/logical evidence supporting the alternative position.
Do not hedge or apologize. ...`
```

The **NLI probe** is the analogue of the NLI contradiction model, producing the
consistency score and listing concrete contradictions:

```go
// "If the agent cleanly argued alternate perspectives without inventing
// contradictory facts, score consistency high (0.8-1.0).
// If the agent directly contradicted its own factual claims, score
// consistency low (0.0-0.4)."
```

## 6. Divergences and extensions

gaslit is a *diagnostic for debates*, not a fact-checking pipeline, so it
extends PCC in three ways:

1. **Drift as a first-class signal.** Before the CCI-style probe runs, gaslit
   measures *position drift* under cross-examination (flips, confidence
   erosion, and a distribution-shift metric like `TVDRift`/`KLDistance`/
   `JSDistance`). The paper's quadrants operate on a single static claim;
   gaslit's quadrants operate on agents that have already been pressured.
2. **Influence attribution.** When agents change their minds, gaslit explains
   *who* swayed whom (`ChampionAttribution`, `ShiftWeightedAttribution`,
   `CorrelationAttribution`) — attribution is the debate analogue of the
   paper's "why retrieve" analysis.
3. **Cheap, deterministic consistency.** `LexicalScorer` approximates the NLI
   model with Jaccard overlap for zero extra model calls, trading fidelity for
   cost — useful when the paper's full NLI machinery is overkill.

The shared intellectual core: **confidence alone is not enough to trust an
agent; you must also test whether its reasoning holds up under adversarial
assumptions.** High confidence + high consistency ⇒ trust (PEAK). High
confidence + fragile reasoning ⇒ precisely the *overconfident hallucination*
the paper targets (HALLUCINATING).

## 7. Reading order

1. [how-it-works.md](./how-it-works.md) — the runtime pipeline.
2. [usage-guide.md](./usage-guide.md) — how to configure thresholds, scorers,
   grill strategies, and read the quadrants.
3. [file-structure.md](./file-structure.md) — where each piece lives
   (`cci.go`, `drift.go`, `grill.go`, `prompts.go`).
4. This document — the theory behind why the quadrant grid is the right
   diagnostic.
