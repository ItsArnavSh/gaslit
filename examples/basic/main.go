package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ItsArnavSh/gaslit"
	"github.com/cloudwego/eino/adk"
)

func main() {
	ctx := context.Background()
	agents, err := GetHistoryPanelAgents(ctx)
	if err != nil {
		log.Fatal(err)
		return
	}
	supervisorModel, err := GetGroqModel(ctx, "openai/gpt-oss-120b")
	if err != nil {
		log.Fatalf("failed to create supervisor model: %v", err)
	}

	cfg := gaslit.PresetDefault()
	cfg.SupervisorModel = supervisorModel
	adkAgents := make([]adk.Agent, len(agents))
	for i, a := range agents {
		adkAgents[i] = a
	}
	cfg.Agents = adkAgents

	engine, err := gaslit.New(ctx, cfg,
		gaslit.WithPacingDelay(3*time.Second),
		gaslit.WithVerbose(true),
		gaslit.WithConcurrency(2),
		gaslit.WithGrillStrategy(gaslit.GrillAllDrifted),
	)
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
}
