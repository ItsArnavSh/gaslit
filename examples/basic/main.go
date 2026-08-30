package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/ItsArnavSh/gaslit"
	"github.com/cloudwego/eino/adk"
)

func main() {
	final := flag.Bool("final", false, "use the LLM consistency scorer for the final benchmark (default: LexicalScorer for debugging)")
	flag.Parse()

	ctx := context.Background()
	agents, err := GetHistoryPanelAgents(ctx)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Strong model drives the moderator and the debaters.
	moderatorModel, err := GetGroqModel(ctx, ModelStrong)
	if err != nil {
		log.Fatalf("failed to create moderator model: %v", err)
	}
	// Cheap model runs the NLI consistency check — a much simpler task.
	scoringModel, err := GetGroqModel(ctx, ModelCheap)
	if err != nil {
		log.Fatalf("failed to create consistency model: %v", err)
	}

	cfg := gaslit.PresetDefault()
	cfg.SupervisorModel = moderatorModel
	cfg.ConsistencyModel = scoringModel
	adkAgents := make([]adk.Agent, len(agents))
	for i, a := range agents {
		adkAgents[i] = a
	}
	cfg.Agents = adkAgents

	// Cap work per debate up front: a bug (e.g. an infinite retry loop) can't
	// blow the budget while the run is unattended.
	opts := []gaslit.Option{
		gaslit.WithBudget(gaslit.CostBudget{
			MaxCalls:  200,
			MaxTokens: 200_000,
			Deadline:  10 * time.Minute,
		}),
		gaslit.WithPacingDelay(500 * time.Millisecond),
		gaslit.WithVerbose(true),
		gaslit.WithConcurrency(2),
		gaslit.WithGrillStrategy(gaslit.GrillAllDrifted),
	}

	if *final {
		// Final benchmark: the default LLM scorer already uses
		// cfg.ConsistencyModel (the cheap model).
		log.Println("using LLM consistency scorer over ConsistencyModel (final run)")
	} else {
		// Debugging the harness: deterministic scorer, zero extra model calls.
		opts = append(opts, gaslit.WithLexicalScorer())
		log.Println("using LexicalScorer (debug) — pass -final for the LLM benchmark")
	}

	engine, err := gaslit.New(ctx, cfg, opts...)
	if err != nil {
		log.Fatalf("failed to init gaslit: %v", err)
	}

	question := "What was the single most decisive factor in the Western Roman Empire's collapse?"
	options := []string{
		"Economic exhaustion from hyperinflation, currency debasement, and tax base flight",
		"Military breakdown and over-reliance on untrustworthy foederati/barbarian mercenaries",
		"Internal cultural and religious transformations eroding traditional civic virtue",
		"A succession of catastrophic plagues crippling the agricultural workforce",
	}

	summary, err := engine.Debate(ctx, question, options)
	if err != nil {
		log.Fatalf("Debate execution failed: %v", err)
	}

	// Cache the run so scoring/quadrant logic can be re-run without fresh
	// baseline/debate model calls.
	if err := gaslit.SaveSummaryFile("debate_summary.json", summary); err != nil {
		log.Printf("warning: failed to save summary: %v", err)
	}

	fmt.Println("\n================ FINAL REPORT ================")
	fmt.Printf("Outcome: %s\n", summary.OverallOutcome)
	fmt.Printf("Drift Detected: %t\n", summary.DriftVerdict.HasDrift)
	fmt.Printf("Winner: %s\n", summary.Winner)

	for name, diag := range summary.Diagnostics {
		fmt.Printf("\n[Diagnostic: %s]\n", name)
		fmt.Printf("• Quadrant   : %s\n", diag.Quadrant)
		fmt.Printf("• Confidence : %.2f\n", diag.ConfidenceScore)
		fmt.Printf("• Consistency: %.2f\n", diag.ConsistencyScore)
		fmt.Printf("• Action     : %s\n", diag.Recommendation)
	}

	// Re-run only the scoring/quadrant code against the cached JSON.
	reloadAndRescore(ctx, engine)
}

func reloadAndRescore(ctx context.Context, engine *gaslit.Gaslit) {
	loaded, err := gaslit.LoadSummaryFile("debate_summary.json")
	if err != nil {
		fmt.Printf("\n[RE-SCORE] could not load cached summary: %v\n", err)
		return
	}

	// Pure re-tally: zero model calls; tune thresholds/quadrant logic here.
	gaslit.ReclassifySummary(loaded, gaslit.Thresholds{Confidence: 0.55, Consistency: 0.60})
	fmt.Printf("\n[RE-SCORE] reclassified %d diagnostics from cache\n", len(loaded.Diagnostics))

	// Model-backed re-score over the stored R1/R2 — no new debate.
	if _, err := engine.Rescore(ctx, loaded); err != nil {
		fmt.Printf("[RE-SCORE] rescore failed: %v\n", err)
		return
	}
	fmt.Printf("[RE-SCORE] rescored %d diagnostics against stored arguments\n", len(loaded.Diagnostics))
}
