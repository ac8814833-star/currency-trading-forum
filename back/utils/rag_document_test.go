package utils

import (
	"back/models"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func TestBuildArticleIndexHashChangesWithSearchableContent(t *testing.T) {
	article := models.Article{Title: "美元走势", Preview: "周度复盘", Content: "美元指数上涨"}
	before := BuildArticleIndexHash(article)

	article.Content = "美元指数回落"
	after := BuildArticleIndexHash(article)

	if before == after {
		t.Fatal("content hash must change when the article content changes")
	}
}

func TestBuildArticleIndexChunksPreservesArticleContext(t *testing.T) {
	article := models.Article{
		Title:    "美元兑人民币走势",
		Preview:  "汇率复盘",
		Username: "trader",
		Content:  "今日美元兑人民币波动收窄。",
	}

	chunks := BuildArticleIndexChunks(article)
	if len(chunks) == 0 {
		t.Fatal("expected at least one index chunk")
	}

	content := chunks[0].Content
	if !strings.Contains(content, "美元兑人民币走势") || !strings.Contains(content, "今日美元兑人民币") {
		t.Fatalf("chunk does not contain the expected article context: %q", content)
	}
}

func TestGenerateRAGAnswerWithoutHitsDoesNotRequireModel(t *testing.T) {
	answer, err := GenerateRAGAnswer("没有匹配内容怎么办？", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer != "根据现有文章内容无法确定。" {
		t.Fatalf("unexpected no-context answer: %q", answer)
	}
}

func TestBuildParentChildChunksUsesStableLocalParents(t *testing.T) {
	article := models.Article{
		Model:      gorm.Model{ID: 42},
		RagVersion: 3,
		Title:      "长期汇率分析",
		Preview:    "测试局部父段",
		Content: strings.Repeat("第一段分析。", 180) +
			"\n\n局部父段B的关键结论。" + strings.Repeat("第二段分析。", 180),
	}

	chunks := buildParentChildChunks(article)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple child chunks, got %d", len(chunks))
	}
	parentIDs := make(map[string]struct{})
	foundLateParent := false
	for _, chunk := range chunks {
		if chunk.DocType != "child" {
			t.Fatalf("only child chunks should be indexed, got %q", chunk.DocType)
		}
		if chunk.ParentID == "" || chunk.ParentContent == "" {
			t.Fatalf("child missing stable parent metadata: %+v", chunk)
		}
		if !strings.Contains(chunk.Content, "长期汇率分析") {
			t.Fatalf("child lost article context: %q", chunk.Content)
		}
		parentIDs[chunk.ParentID] = struct{}{}
		if strings.Contains(chunk.ParentContent, "局部父段B的关键结论") {
			foundLateParent = true
		}
	}
	if len(parentIDs) < 2 {
		t.Fatalf("expected more than one local parent, got %v", parentIDs)
	}
	if !foundLateParent {
		t.Fatal("late-section text was not preserved in a local parent")
	}

	again := buildParentChildChunks(article)
	if len(again) != len(chunks) {
		t.Fatalf("chunk layout should be stable: %d != %d", len(again), len(chunks))
	}
	for i := range chunks {
		if chunks[i].ParentID != again[i].ParentID || chunks[i].IDSuffix != again[i].IDSuffix {
			t.Fatalf("parent mapping changed at %d: %+v vs %+v", i, chunks[i], again[i])
		}
	}
}
