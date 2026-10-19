package utils

import (
	"context"
	"testing"
	"time"
)

func TestRunRAGEvaluationCalculatesGroundedMetrics(t *testing.T) {
	suite := RAGEvalSuite{Cases: []RAGEvalCase{
		{ID: "answerable", Question: "问题一", ExpectedArticleIDs: []uint{7}, ExpectAnswerable: true},
		{ID: "no-answer", Question: "问题二", ExpectAnswerable: false},
	}}
	runner := func(_ context.Context, question string) (string, []RAGHit, time.Duration, time.Duration, error) {
		if question == "问题一" {
			return "有依据的回答[文章 7]", []RAGHit{{ArticleID: 7, Content: "证据"}}, 10 * time.Millisecond, 20 * time.Millisecond, nil
		}
		return noAnswerRAGResponse, nil, 5 * time.Millisecond, 0, nil
	}

	report := RunRAGEvaluation(context.Background(), suite, RAGEvalPricing{
		InputPricePerMillionTokens:  1,
		OutputPricePerMillionTokens: 2,
	}, runner)
	if report.Summary.RecallAtK != 1 || report.Summary.CitationCorrectness != 1 || report.Summary.NoAnswerAccuracy != 1 {
		t.Fatalf("unexpected evaluation summary: %+v", report.Summary)
	}
	if report.Summary.EstimatedTotalCostCNY <= 0 {
		t.Fatalf("expected a positive estimated cost, got %f", report.Summary.EstimatedTotalCostCNY)
	}
}
