package main

import (
	"back/config"
	"back/utils"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	casesPath := flag.String("cases", "testdata/rag_eval_cases.json", "path to the labelled RAG evaluation suite")
	outputPath := flag.String("out", "", "optional JSON report output path")
	inputPrice := flag.Float64("input-price-per-million-tokens", 0, "optional CNY price per million estimated input tokens")
	outputPrice := flag.Float64("output-price-per-million-tokens", 0, "optional CNY price per million estimated output tokens")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall evaluation timeout")
	flag.Parse()

	suite, err := utils.LoadRAGEvalSuite(*casesPath)
	if err != nil {
		fatal(err)
	}

	config.InitConfig()
	defer func() { _ = config.CloseRabbitMQ() }()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report := utils.RunRAGEvaluation(ctx, suite, utils.RAGEvalPricing{
		InputPricePerMillionTokens:  *inputPrice,
		OutputPricePerMillionTokens: *outputPrice,
	}, utils.LiveRAGEvalRunner)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("RAG evaluation report written to %s\n", *outputPath)
		return
	}
	fmt.Println(string(data))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "rag-eval:", err)
	os.Exit(1)
}
