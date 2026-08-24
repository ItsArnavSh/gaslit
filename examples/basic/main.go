package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ItsArnavSh/gaslight"
	"github.com/cloudwego/eino/adk"
)

func main() {
	ctx := context.Background()
	agents, err := GetHistoryPanelAgents(ctx)
	if err != nil {
		log.Fatal(err)
		return
	}
	adkAgents := make([]adk.Agent, len(agents))
	for i, a := range agents {
		adkAgents[i] = a
	}
	supervisorModel, err := GetGroqModel(ctx, "openai/gpt-oss-120b")
	if err != nil {
		log.Fatalf("failed to create supervisor model: %v", err)
	}
	gaslight, err := gaslit.Init(ctx, supervisorModel, adkAgents)
	if err != nil {
		log.Fatalf("failed to init gaslit: %v", err)
	}

	// 5. Define the question and ordered options
	question := "What was the single most decisive factor in the Western Roman Empire's collapse?"
	options := []string{
		"Economic exhaustion from hyperinflation, currency debasement, and tax base flight",
		"Military breakdown and over-reliance on untrustworthy foederati/barbarian mercenaries",
		"Internal cultural and religious transformations eroding traditional civic virtue",
		"A succession of catastrophic plagues crippling the agricultural workforce",
	}

	// 6. Run the baseline evaluation, supervised cross-examination, and print confidence shifts
	verbose := true
	summary, err := gaslight.Debate(ctx, question, options, verbose)
	if err != nil {
		log.Fatalf("Debate execution failed: %v", err)
	}

	// 5. Inspect or serialize the final output
	fmt.Println("\n================ FINAL REPORT ================")
	fmt.Printf("Outcome: %s\n", summary.OverallOutcome)
	fmt.Printf("Drift Detected: %t\n", summary.DriftVerdict.HasDrift)

	for name, diag := range summary.Diagnostics {
		fmt.Printf("\n[Diagnostic: %s]\n", name)
		fmt.Printf("• Quadrant   : %s\n", diag.Quadrant)
		fmt.Printf("• Confidence : %.2f\n", diag.ConfidenceScore)
		fmt.Printf("• Consistency: %.2f\n", diag.ConsistencyScore)
		fmt.Printf("• Action     : %s\n", diag.Recommendation)
	}
}
