package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ItsArnavSh/gaslit/datatest"
	"github.com/ItsArnavSh/gaslit/examples/internal/groqmodel"
)

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var (
		modelID   = flag.String("qa-model", "groq/compound-mini", "QA model id (e.g. groq/compound-mini, groq/allam-2-7b)")
		agentModel = flag.String("agent-model", "groq/compound-mini", "Agent model id")
		outMd     = flag.String("out-md", "analysis.md", "Markdown report path")
		outJSON   = flag.String("out-json", "analysis.json", "JSON report path")
		qbOut     = flag.String("qb-out", "questions.json", "Question bank output path")
		perChap   = flag.Int("per-chapter", 2, "Questions per chapter")
		total     = flag.Int("total", 0, "Total questions (overrides per-chapter if >0)")
		maxChap   = flag.Int("max-chapters", 5, "Max chapters to sample")
		optsN     = flag.Int("options", 4, "Options per question")
		difficulty = flag.String("difficulty", "hard", "Difficulty: easy/medium/hard/mixed")
	)
	flag.Parse()

	suite := datatest.New(ctx, datatest.WithLogger(logger))

	// Add sources from local epubs if present; also allow URLs via env/flags? simple: add local test epubs
	candidates := []struct{ name, path string }{
		{"republic", "/tmp/opencode/republic.epub"},
		{"meditations", "/tmp/opencode/meditations.epub"},
		{"prince", "/tmp/opencode/prince.epub"},
	}
	for _, c := range candidates {
		if _, err := os.Stat(c.path); err == nil {
			if err := suite.AddSource(c.name, c.path); err != nil {
				logger.Warn("add source failed", "name", c.name, "err", err)
			} else {
				logger.Info("added source", "name", c.name)
			}
		}
	}

	if len(suite.Sources()) == 0 {
		logger.Warn("no sources added; pass URLs or paths via code")
	}

	// models
	qaModel, err := groqmodel.GetGroqModel(ctx, *modelID)
	if err != nil {
		logger.Error("qa model", "err", err)
		os.Exit(1)
	}
	agModel, err := groqmodel.GetGroqModel(ctx, *agentModel)
	if err != nil {
		logger.Error("agent model", "err", err)
		os.Exit(1)
	}

	// add agents
	if _, err := suite.AddAgent(ctx, "RAG-Agent-1", "You are a careful, evidence-based reader. Use the provided sources. Answer precisely.", nil, agModel); err != nil {
		logger.Error("add agent", "err", err)
		os.Exit(1)
	}

	// generate
	qcfg := &datatest.QuestionnaireConfig{
		PerChapter:         *perChap,
		Total:              *total,
		MaxChapters:        *maxChap,
		Options:            *optsN,
		Difficulty:         *difficulty,
		IncludeExplanation: false,
		MaxChapterChars:    16000,
		Language:           "English",
	}
	bank, err := suite.GenerateQuestionnaire(ctx, qaModel, qcfg)
	if err != nil {
		logger.Error("generate", "err", err)
		os.Exit(1)
	}
	if err := datatest.SaveQuestionBank(*qbOut, bank); err != nil {
		logger.Warn("save bank", "err", err)
	}
	logger.Info("generated", "questions", len(bank.Questions))

	// run
	res, err := suite.Run(ctx, bank, datatest.RunConfig{Mode: datatest.ModeAnswerOnly, Verbose: false})
	if err != nil {
		logger.Error("run", "err", err)
		os.Exit(1)
	}
	logger.Info("run complete", "mode", res.Mode, "results", len(res.Results), "took", time.Since(res.StartedAt))

	// analyze/report
	an := suite.Analyze(res)
	if *outMd != "" {
		if err := an.Save(*outMd); err != nil {
			logger.Warn("save md", "err", err)
		} else {
			logger.Info("wrote", "path", *outMd)
		}
	}
	if *outJSON != "" {
		if err := an.SaveJSON(*outJSON); err != nil {
			logger.Warn("save json", "err", err)
		} else {
			logger.Info("wrote", "path", *outJSON)
		}
	}
	fmt.Println(an.Markdown())
}
