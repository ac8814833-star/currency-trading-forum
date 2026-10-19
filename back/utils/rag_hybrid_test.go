package utils

import "testing"

func TestSelectRAGHitsByArticleOrderUsesFusedArticleRank(t *testing.T) {
	hits := []RAGHit{
		{ChunkID: "a-1", ArticleID: 1, Distance: 0.01},
		{ChunkID: "a-2", ArticleID: 1, Distance: 0.02},
		{ChunkID: "b-1", ArticleID: 2, Distance: 0.50},
		{ChunkID: "b-2", ArticleID: 2, Distance: 0.60},
	}
	selected := selectRAGHitsByArticleOrder(hits, []uint{2, 1}, 3, 2)
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected hits, got %d", len(selected))
	}
	if selected[0].ArticleID != 2 || selected[1].ArticleID != 2 || selected[2].ArticleID != 1 {
		t.Fatalf("fused article order not respected: %+v", selected)
	}
}
