package utils

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRAGPromptContextDeduplicatesParentsAndAddsDate(t *testing.T) {
	now := time.Date(2026, time.April, 19, 12, 0, 0, 0, time.Local)
	hits := []RAGHit{
		{ArticleID: 7, Title: "货币体系", Content: "子块一", ParentID: "p-1", ParentContent: "局部父段内容", ArticleUpdatedAt: &now},
		{ArticleID: 7, Title: "货币体系", Content: "子块二", ParentID: "p-1", ParentContent: "局部父段内容", ArticleUpdatedAt: &now},
	}

	context, cited := buildRAGPromptContext(hits)
	if strings.Count(context, "局部父段内容") != 1 {
		t.Fatalf("parent context should appear once, got %q", context)
	}
	if !strings.Contains(context, "最后更新：2026-04-19") {
		t.Fatalf("context is missing article date: %q", context)
	}
	if len(cited) != 1 {
		t.Fatalf("expected one cited parent, got %d", len(cited))
	}
}

func TestNormalizeRAGCitationsAndTimeSensitiveQuestions(t *testing.T) {
	hits := []RAGHit{{ArticleID: 7}, {ArticleID: 8}}
	answer := normalizeRAGCitations("结论[文章 7]，伪造引用[文章 99]。", hits)
	if !strings.Contains(answer, "[文章 7]") || strings.Contains(answer, "[文章 99]") {
		t.Fatalf("unexpected normalized citations: %q", answer)
	}

	answer = normalizeRAGCitations("没有显式引用。", hits)
	if !strings.Contains(answer, "参考来源：[文章 7]、[文章 8]") {
		t.Fatalf("missing citation footer: %q", answer)
	}
	if !isTimeSensitiveRAGQuestion("今天美元兑人民币的最新走势如何？") {
		t.Fatal("expected time-sensitive question to be detected")
	}
	if isTimeSensitiveRAGQuestion("数字钱包有哪些特点？") {
		t.Fatal("ordinary knowledge question should not be time-sensitive")
	}
}
