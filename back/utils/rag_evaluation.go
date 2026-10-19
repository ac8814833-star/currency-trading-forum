package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// RAGEvalCase is a labelled question used to measure retrieval and answer
// grounding against the forum corpus. expected_article_ids are the supporting
// documents, not necessarily the only documents that may be retrieved.
type RAGEvalCase struct {
	ID                 string `json:"id"`
	Question           string `json:"question"`
	ExpectedArticleIDs []uint `json:"expected_article_ids,omitempty"`
	ExpectAnswerable   bool   `json:"expect_answerable"`
	Notes              string `json:"notes,omitempty"`
}

type RAGEvalSuite struct {
	Version     int           `json:"version"`
	Description string        `json:"description"`
	Cases       []RAGEvalCase `json:"cases"`
}

type RAGEvalPricing struct {
	// Prices are CNY per million estimated tokens. They are optional because
	// provider billing is authoritative; the report labels its value estimated.
	InputPricePerMillionTokens  float64 `json:"input_price_per_million_tokens"`
	OutputPricePerMillionTokens float64 `json:"output_price_per_million_tokens"`
}

func (p RAGEvalPricing) Configured() bool {
	return p.InputPricePerMillionTokens > 0 || p.OutputPricePerMillionTokens > 0
}

type RAGEvalCaseResult struct {
	ID                    string  `json:"id"`
	Question              string  `json:"question"`
	ExpectedArticleIDs    []uint  `json:"expected_article_ids,omitempty"`
	ExpectAnswerable      bool    `json:"expect_answerable"`
	HitArticleIDs         []uint  `json:"hit_article_ids,omitempty"`
	CitationArticleIDs    []uint  `json:"citation_article_ids,omitempty"`
	RetrievalHit          bool    `json:"retrieval_hit"`
	CitationsGrounded     bool    `json:"citations_grounded"`
	ExpectedCitation      bool    `json:"expected_citation"`
	NoAnswerCorrect       bool    `json:"no_answer_correct"`
	RetrievalLatencyMS    int64   `json:"retrieval_latency_ms"`
	GenerationLatencyMS   int64   `json:"generation_latency_ms"`
	TotalLatencyMS        int64   `json:"total_latency_ms"`
	EstimatedInputTokens  int     `json:"estimated_input_tokens"`
	EstimatedOutputTokens int     `json:"estimated_output_tokens"`
	EstimatedCostCNY      float64 `json:"estimated_cost_cny"`
	Answer                string  `json:"answer"`
	Error                 string  `json:"error,omitempty"`
}

type RAGEvalSummary struct {
	TotalCases            int     `json:"total_cases"`
	AnswerableCases       int     `json:"answerable_cases"`
	NoAnswerCases         int     `json:"no_answer_cases"`
	CasesWithErrors       int     `json:"cases_with_errors"`
	RetrievalHitCases     int     `json:"retrieval_hit_cases"`
	CitationCorrectCases  int     `json:"citation_correct_cases"`
	NoAnswerCorrectCases  int     `json:"no_answer_correct_cases"`
	RecallAtK             float64 `json:"recall_at_k"`
	CitationCorrectness   float64 `json:"citation_correctness"`
	NoAnswerAccuracy      float64 `json:"no_answer_accuracy"`
	AverageTotalLatencyMS float64 `json:"average_total_latency_ms"`
	P50TotalLatencyMS     int64   `json:"p50_total_latency_ms"`
	P95TotalLatencyMS     int64   `json:"p95_total_latency_ms"`
	EstimatedTotalCostCNY float64 `json:"estimated_total_cost_cny"`
}

type RAGEvalReport struct {
	GeneratedAt      time.Time           `json:"generated_at"`
	SuiteVersion     int                 `json:"suite_version"`
	SuiteDescription string              `json:"suite_description"`
	Pricing          RAGEvalPricing      `json:"pricing"`
	CostEstimated    bool                `json:"cost_estimated"`
	Summary          RAGEvalSummary      `json:"summary"`
	Results          []RAGEvalCaseResult `json:"results"`
}

// RAGEvalRunner can be replaced with a fake in tests or a remote API client in
// CI. Its duration values deliberately separate retrieval from generation.
type RAGEvalRunner func(ctx context.Context, question string) (answer string, hits []RAGHit, retrieval time.Duration, generation time.Duration, err error)

func LoadRAGEvalSuite(path string) (RAGEvalSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RAGEvalSuite{}, err
	}
	var suite RAGEvalSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return RAGEvalSuite{}, fmt.Errorf("parse RAG evaluation suite: %w", err)
	}
	if len(suite.Cases) == 0 {
		return RAGEvalSuite{}, fmt.Errorf("RAG evaluation suite has no cases")
	}
	for i, item := range suite.Cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Question) == "" {
			return RAGEvalSuite{}, fmt.Errorf("RAG evaluation case %d is missing id or question", i)
		}
		if item.ExpectAnswerable && len(item.ExpectedArticleIDs) == 0 {
			return RAGEvalSuite{}, fmt.Errorf("answerable RAG evaluation case %q has no expected article", item.ID)
		}
	}
	return suite, nil
}

func RunRAGEvaluation(ctx context.Context, suite RAGEvalSuite, pricing RAGEvalPricing, runner RAGEvalRunner) RAGEvalReport {
	report := RAGEvalReport{
		GeneratedAt:      time.Now().UTC(),
		SuiteVersion:     suite.Version,
		SuiteDescription: suite.Description,
		Pricing:          pricing,
		CostEstimated:    pricing.Configured(),
		Results:          make([]RAGEvalCaseResult, 0, len(suite.Cases)),
	}
	for _, item := range suite.Cases {
		startedAt := time.Now()
		answer, hits, retrievalLatency, generationLatency, err := runner(ctx, item.Question)
		result := RAGEvalCaseResult{
			ID:                  item.ID,
			Question:            item.Question,
			ExpectedArticleIDs:  append([]uint(nil), item.ExpectedArticleIDs...),
			ExpectAnswerable:    item.ExpectAnswerable,
			RetrievalLatencyMS:  retrievalLatency.Milliseconds(),
			GenerationLatencyMS: generationLatency.Milliseconds(),
			TotalLatencyMS:      time.Since(startedAt).Milliseconds(),
			Answer:              answer,
		}
		if err != nil {
			result.Error = err.Error()
			report.Results = append(report.Results, result)
			continue
		}

		result.HitArticleIDs = uniqueRAGHitArticleIDs(hits)
		result.CitationArticleIDs = extractRAGCitationIDs(answer)
		result.RetrievalHit = containsAnyArticleID(result.HitArticleIDs, item.ExpectedArticleIDs)
		result.CitationsGrounded = citationsAreGrounded(result.CitationArticleIDs, result.HitArticleIDs)
		result.ExpectedCitation = containsAnyArticleID(result.CitationArticleIDs, item.ExpectedArticleIDs)
		result.NoAnswerCorrect = !item.ExpectAnswerable && isRAGNoAnswer(answer)

		contextText, _ := buildRAGPromptContext(hits)
		result.EstimatedInputTokens = estimateRAGTokens(item.Question) + estimateRAGTokens(contextText) + 220
		result.EstimatedOutputTokens = estimateRAGTokens(answer)
		result.EstimatedCostCNY = estimatedRAGCost(result.EstimatedInputTokens, result.EstimatedOutputTokens, pricing)
		report.Results = append(report.Results, result)
	}
	report.Summary = summarizeRAGEvaluation(report.Results)
	return report
}

func LiveRAGEvalRunner(ctx context.Context, question string) (string, []RAGHit, time.Duration, time.Duration, error) {
	retrievalStarted := time.Now()
	hits, err := RetrieveRAGHits(question, 0)
	retrievalLatency := time.Since(retrievalStarted)
	if err != nil {
		return "", nil, retrievalLatency, 0, err
	}
	generationStarted := time.Now()
	answer, err := GenerateRAGAnswer(question, hits)
	return answer, hits, retrievalLatency, time.Since(generationStarted), err
}

func summarizeRAGEvaluation(results []RAGEvalCaseResult) RAGEvalSummary {
	summary := RAGEvalSummary{TotalCases: len(results)}
	latencies := make([]int64, 0, len(results))
	var latencyTotal int64
	for _, result := range results {
		summary.EstimatedTotalCostCNY += result.EstimatedCostCNY
		if result.Error != "" {
			summary.CasesWithErrors++
			continue
		}
		latencies = append(latencies, result.TotalLatencyMS)
		latencyTotal += result.TotalLatencyMS
		if result.ExpectAnswerable {
			summary.AnswerableCases++
			if result.RetrievalHit {
				summary.RetrievalHitCases++
			}
			if result.CitationsGrounded && result.ExpectedCitation {
				summary.CitationCorrectCases++
			}
		} else {
			summary.NoAnswerCases++
			if result.NoAnswerCorrect {
				summary.NoAnswerCorrectCases++
			}
		}
	}
	if summary.AnswerableCases > 0 {
		summary.RecallAtK = float64(summary.RetrievalHitCases) / float64(summary.AnswerableCases)
		summary.CitationCorrectness = float64(summary.CitationCorrectCases) / float64(summary.AnswerableCases)
	}
	if summary.NoAnswerCases > 0 {
		summary.NoAnswerAccuracy = float64(summary.NoAnswerCorrectCases) / float64(summary.NoAnswerCases)
	}
	if len(latencies) > 0 {
		summary.AverageTotalLatencyMS = float64(latencyTotal) / float64(len(latencies))
		summary.P50TotalLatencyMS = percentileLatency(latencies, 0.50)
		summary.P95TotalLatencyMS = percentileLatency(latencies, 0.95)
	}
	return summary
}

func uniqueRAGHitArticleIDs(hits []RAGHit) []uint {
	ids := make([]uint, 0, len(hits))
	seen := make(map[uint]struct{}, len(hits))
	for _, hit := range hits {
		if hit.ArticleID == 0 {
			continue
		}
		if _, exists := seen[hit.ArticleID]; exists {
			continue
		}
		seen[hit.ArticleID] = struct{}{}
		ids = append(ids, hit.ArticleID)
	}
	return ids
}

func containsAnyArticleID(actual, expected []uint) bool {
	if len(actual) == 0 || len(expected) == 0 {
		return false
	}
	seen := make(map[uint]struct{}, len(actual))
	for _, id := range actual {
		seen[id] = struct{}{}
	}
	for _, id := range expected {
		if _, exists := seen[id]; exists {
			return true
		}
	}
	return false
}

func citationsAreGrounded(citations, hitIDs []uint) bool {
	if len(citations) == 0 {
		return false
	}
	allowed := make(map[uint]struct{}, len(hitIDs))
	for _, articleID := range hitIDs {
		allowed[articleID] = struct{}{}
	}
	for _, articleID := range citations {
		if _, exists := allowed[articleID]; !exists {
			return false
		}
	}
	return true
}

func estimateRAGTokens(text string) int {
	asciiRunes, nonASCIIRunes := 0, 0
	for _, r := range text {
		if r <= 127 {
			asciiRunes++
		} else {
			nonASCIIRunes++
		}
	}
	return nonASCIIRunes + int(math.Ceil(float64(asciiRunes)/4.0))
}

func estimatedRAGCost(inputTokens, outputTokens int, pricing RAGEvalPricing) float64 {
	return float64(inputTokens)/1_000_000*pricing.InputPricePerMillionTokens +
		float64(outputTokens)/1_000_000*pricing.OutputPricePerMillionTokens
}

func percentileLatency(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
