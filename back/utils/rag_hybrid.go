package utils

import (
	"back/config"
	"back/global"
	"back/models"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

func RetrieveRAGHits(question string, topK int) ([]RAGHit, error) {
	topK = normalizedRAGTopK(topK)

	// Stage 1: broad document recall. Vector results are collapsed to article
	// ranks and fused with ES/MySQL keyword ranks via RRF.
	queryVectors, vecErr := EmbedTexts([]string{question})
	var vecHits []RAGHit
	if vecErr == nil {
		vecHits, vecErr = queryRelevantChunksByVectors(queryVectors, ragCandidateChunkLimit(topK), nil)
		if vecErr == nil {
			// Remove deleted, stale-version, and pre-schema-upgrade chunks before
			// they can influence the article-level RRF candidate set.
			vecHits = FilterAliveArticleHits(vecHits)
		}
	}
	if vecErr != nil {
		logrus.Warnf("rag vector search failed: %v", vecErr)
	}
	keywordLimit := ragCandidateArticleLimit()
	var keywordIDs []uint
	var keywordErr error
	if config.AppConfig == nil || config.AppConfig.RAG.BM25Enabled {
		if global.ESClient != nil {
			var esHits []esHit
			esHits, keywordErr = ESBM25Search(question, keywordLimit)
			if keywordErr == nil {
				for _, hit := range esHits {
					keywordIDs = append(keywordIDs, hit.ArticleID)
				}
			} else {
				logrus.Warnf("rag ES search failed, using MySQL: %v", keywordErr)
			}
		}
		if global.ESClient == nil || keywordErr != nil {
			keywordIDs, keywordErr = CandidateArticleIDsByBM25(question, keywordLimit)
		}
	}
	if keywordErr != nil {
		logrus.Warnf("rag keyword search failed: %v", keywordErr)
	}
	if vecErr != nil && (keywordErr != nil || len(keywordIDs) == 0) {
		return nil, fmt.Errorf("rag retrieval unavailable: vector: %v; keyword: %v", vecErr, keywordErr)
	}

	keywordRanks := make([]esHit, 0, len(keywordIDs))
	for _, id := range keywordIDs {
		keywordRanks = append(keywordRanks, esHit{ArticleID: id})
	}
	articleOrder := RRFFusion(vecHits, keywordRanks)
	articleCandidates := takeArticleCandidates(articleOrder, keywordLimit)

	// Stage 2: ask Chroma for the best child chunks only within the fused article
	// set. This lets a strong BM25-only article contribute its relevant middle
	// section instead of falling back to the first 800 characters of the article.
	if vecErr == nil && len(articleCandidates) > 0 {
		stageTwoHits, err := queryRelevantChunksByVectors(queryVectors, ragFinalCandidateLimit(topK), articleCandidates)
		if err != nil {
			logrus.Warnf("rag second-stage vector search failed, using first-stage hits: %v", err)
		} else {
			stageTwoHits = FilterAliveArticleHits(stageTwoHits)
			if len(stageTwoHits) > 0 {
				source := "vector"
				if len(keywordIDs) > 0 {
					source = "hybrid"
				}
				stageTwoHits = setRAGHitSource(stageTwoHits, source)
				selected := selectRAGHitsByArticleOrder(stageTwoHits, articleOrder, ragFinalCandidateLimit(topK), ragMaxChunksPerArticle())
				if len(selected) > 0 {
					return finalizeRAGHits(question, selected, topK), nil
				}
			}
		}
	}

	// A vector service outage should not turn a keyword hit into a hard failure.
	// Use the first-stage hits if available, then fall back to a bounded article
	// excerpt only when no exact child chunk can be fetched.
	firstStage := FilterAliveArticleHits(vecHits)
	if len(firstStage) > 0 {
		source := "vector"
		if len(keywordIDs) > 0 {
			source = "hybrid"
		}
		firstStage = setRAGHitSource(firstStage, source)
		selected := selectRAGHitsByArticleOrder(firstStage, articleOrder, ragFinalCandidateLimit(topK), ragMaxChunksPerArticle())
		if len(selected) > 0 {
			return finalizeRAGHits(question, selected, topK), nil
		}
	}

	keywordFallback := FilterAliveArticleHits(buildKeywordFallbackHits(articleCandidates, ragFinalCandidateLimit(topK)))
	if len(keywordFallback) > 0 {
		return finalizeRAGHits(question, keywordFallback, topK), nil
	}
	return nil, nil
}

func normalizedRAGTopK(topK int) int {
	if topK > 0 {
		return topK
	}
	if config.AppConfig != nil && config.AppConfig.RAG.TopK > 0 {
		return config.AppConfig.RAG.TopK
	}
	return 5
}

func ragCandidateArticleLimit() int {
	if config.AppConfig != nil && config.AppConfig.RAG.ParentTopN > 0 {
		return config.AppConfig.RAG.ParentTopN
	}
	return 20
}

func ragCandidateChunkLimit(topK int) int {
	limit := topK * 3
	if limit < 12 {
		limit = 12
	}
	return limit
}

func ragFinalCandidateLimit(topK int) int {
	if config.AppConfig != nil && config.AppConfig.RAG.RerankEnabled {
		return ragCandidateChunkLimit(topK)
	}
	return topK
}

func ragMaxChunksPerArticle() int {
	if config.AppConfig != nil && config.AppConfig.RAG.MaxChunksPerArticle > 0 {
		return config.AppConfig.RAG.MaxChunksPerArticle
	}
	return 3
}

func takeArticleCandidates(order []uint, limit int) []uint {
	if limit <= 0 || len(order) <= limit {
		return append([]uint(nil), order...)
	}
	return append([]uint(nil), order[:limit]...)
}

func setRAGHitSource(hits []RAGHit, source string) []RAGHit {
	out := make([]RAGHit, len(hits))
	copy(out, hits)
	for i := range out {
		out[i].Source = source
	}
	return out
}

func selectRAGHitsByArticleOrder(hits []RAGHit, articleOrder []uint, limit, maxPerArticle int) []RAGHit {
	if limit <= 0 || len(hits) == 0 {
		return nil
	}
	if maxPerArticle <= 0 {
		maxPerArticle = 1
	}
	ranks := make(map[uint]int, len(articleOrder))
	for i, articleID := range articleOrder {
		ranks[articleID] = i
	}

	ordered := append([]RAGHit(nil), hits...)
	sort.SliceStable(ordered, func(i, j int) bool {
		rankI, okI := ranks[ordered[i].ArticleID]
		rankJ, okJ := ranks[ordered[j].ArticleID]
		if okI && okJ && rankI != rankJ {
			return rankI < rankJ
		}
		if okI != okJ {
			return okI
		}
		return ordered[i].Distance < ordered[j].Distance
	})

	seenChunks := make(map[string]struct{}, len(ordered))
	perArticle := make(map[uint]int, len(ordered))
	selected := make([]RAGHit, 0, limit)
	for _, hit := range ordered {
		if len(selected) >= limit {
			break
		}
		if hit.ChunkID != "" {
			if _, seen := seenChunks[hit.ChunkID]; seen {
				continue
			}
			seenChunks[hit.ChunkID] = struct{}{}
		}
		if perArticle[hit.ArticleID] >= maxPerArticle {
			continue
		}
		perArticle[hit.ArticleID]++
		selected = append(selected, hit)
	}
	return selected
}

func buildKeywordFallbackHits(articleIDs []uint, limit int) []RAGHit {
	if global.Db == nil || len(articleIDs) == 0 || limit <= 0 {
		return nil
	}
	var articles []models.Article
	if err := global.Db.Where("id IN ?", articleIDs).Find(&articles).Error; err != nil {
		logrus.Warnf("rag keyword fallback article lookup failed: %v", err)
		return nil
	}
	byID := make(map[uint]models.Article, len(articles))
	for _, article := range articles {
		byID[article.ID] = article
	}
	fallback := make([]RAGHit, 0, limit)
	for _, articleID := range articleIDs {
		if len(fallback) >= limit {
			break
		}
		article, ok := byID[articleID]
		if !ok {
			continue
		}
		fallback = append(fallback, RAGHit{
			ChunkID:            fmt.Sprintf("article_%d_keyword", article.ID),
			ArticleID:          article.ID,
			Title:              article.Title,
			Content:            truncateRunes(BuildArticleDocument(article), 1200),
			Source:             "keyword",
			RagVersion:         article.RagVersion,
			IndexSchemaVersion: article.RagIndexSchemaVersion,
		})
	}
	return fallback
}

func finalizeRAGHits(question string, hits []RAGHit, topK int) []RAGHit {
	if len(hits) == 0 {
		return nil
	}
	if config.AppConfig != nil && config.AppConfig.RAG.RerankEnabled && len(hits) > 1 {
		if reranked, err := RerankHits(question, hits); err == nil {
			hits = reranked
		} else {
			logrus.Warnf("rag rerank failed: %v", err)
		}
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return decorateRAGHits(hits)
}

// FilterAliveArticleHits 二次过滤已软删除文章，避免 Chroma 脏数据进入回答
func FilterAliveArticleHits(hits []RAGHit) []RAGHit {
	if len(hits) == 0 || global.Db == nil {
		return hits
	}

	idSet := make(map[uint]struct{}, len(hits))
	ids := make([]uint, 0, len(hits))
	for _, h := range hits {
		if h.ArticleID == 0 {
			continue
		}
		if _, ok := idSet[h.ArticleID]; ok {
			continue
		}
		idSet[h.ArticleID] = struct{}{}
		ids = append(ids, h.ArticleID)
	}
	if len(ids) == 0 {
		return hits
	}

	type row struct {
		ID                 uint      `gorm:"column:id"`
		Title              string    `gorm:"column:title"`
		RagVersion         int       `gorm:"column:rag_version"`
		IndexSchemaVersion int       `gorm:"column:rag_index_schema_version"`
		UpdatedAt          time.Time `gorm:"column:updated_at"`
	}
	var rows []row
	if err := global.Db.Raw(
		"SELECT id, title, rag_version, rag_index_schema_version, updated_at FROM articles WHERE id IN ? AND deleted_at IS NULL",
		ids,
	).Scan(&rows).Error; err != nil {
		logrus.Warnf("FilterAliveArticleHits query failed: %v", err)
		return nil
	}

	alive := make(map[uint]row, len(rows))
	for _, r := range rows {
		alive[r.ID] = r
	}

	out := make([]RAGHit, 0, len(hits))
	for _, h := range hits {
		if h.ArticleID == 0 {
			continue
		}
		if current, ok := alive[h.ArticleID]; ok && (h.Source == "keyword" || (h.RagVersion == current.RagVersion && h.IndexSchemaVersion == current.IndexSchemaVersion)) {
			if current.Title != "" {
				h.Title = current.Title
			}
			updatedAt := current.UpdatedAt
			h.ArticleUpdatedAt = &updatedAt
			h.Excerpt = makeRAGExcerpt(h.Content)
			out = append(out, h)
		}
	}
	return out
}

func AttachParentContexts(hits []RAGHit) []RAGHit {
	if len(hits) == 0 {
		return hits
	}

	out := make([]RAGHit, 0, len(hits))
	for _, hit := range hits {
		parent := strings.TrimSpace(hit.ParentContent)
		child := strings.TrimSpace(hit.Content)
		if parent == "" || child == "" || parent == child {
			out = append(out, hit)
			continue
		}

		hit.Content = parent + "\n\n命中片段：\n" + child
		out = append(out, hit)
	}
	return out
}

func CandidateArticleIDsByBM25(question string, limit int) ([]uint, error) {
	if global.Db == nil {
		return nil, errors.New("db is not initialized")
	}
	q := strings.TrimSpace(question)
	if q == "" {
		return []uint{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	var ids []uint

	type row struct {
		ID uint `gorm:"column:id"`
	}
	var rows []row
	var err error
	if ensureArticleFullTextIndex() {
		err = global.Db.Raw(
			"SELECT id FROM articles WHERE MATCH(title, preview, content) AGAINST (? IN NATURAL LANGUAGE MODE) AND deleted_at IS NULL ORDER BY MATCH(title, preview, content) AGAINST (? IN NATURAL LANGUAGE MODE) DESC LIMIT ?",
			q, q, limit,
		).Scan(&rows).Error
		if err == nil {
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			return ids, nil
		}
	}

	like := "%" + q + "%"
	rows = nil
	if err2 := global.Db.Raw(
		"SELECT id FROM articles WHERE (title LIKE ? OR preview LIKE ? OR content LIKE ?) AND deleted_at IS NULL ORDER BY id DESC LIMIT ?",
		like, like, like, limit,
	).Scan(&rows).Error; err2 != nil {
		return nil, err2
	}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

var (
	fullTextIndexOnce      sync.Once
	fullTextIndexAvailable bool
)

func ensureArticleFullTextIndex() bool {
	if global.Db == nil {
		return false
	}
	fullTextIndexOnce.Do(func() {
		var count int64
		err := global.Db.Raw(
			"SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'articles' AND index_name = 'idx_articles_fulltext'",
		).Scan(&count).Error
		if err != nil {
			logrus.Warnf("check fulltext index failed: %v", err)
			return
		}

		if count == 0 {
			if err := global.Db.Exec("CREATE FULLTEXT INDEX idx_articles_fulltext ON articles(title, preview, content)").Error; err != nil {
				logrus.Warnf("create fulltext index failed, fallback to LIKE: %v", err)
				return
			}
		}
		fullTextIndexAvailable = true
	})
	return fullTextIndexAvailable
}

func MergeAndCapHits(primary []RAGHit, secondary []RAGHit, maxTotal int, maxPerArticle int) []RAGHit {
	if maxTotal <= 0 {
		maxTotal = config.AppConfig.RAG.TopK
		if maxTotal <= 0 {
			maxTotal = 5
		}
	}
	if maxPerArticle <= 0 {
		maxPerArticle = config.AppConfig.RAG.MaxChunksPerArticle
		if maxPerArticle <= 0 {
			maxPerArticle = 3
		}
	}

	seen := make(map[string]struct{}, len(primary)+len(secondary))
	perArticle := make(map[uint]int, 64)
	out := make([]RAGHit, 0, maxTotal)

	appendHit := func(h RAGHit) {
		if len(out) >= maxTotal {
			return
		}
		if _, ok := seen[h.ChunkID]; ok && h.ChunkID != "" {
			return
		}
		if h.ArticleID != 0 && perArticle[h.ArticleID] >= maxPerArticle {
			return
		}
		if h.ChunkID != "" {
			seen[h.ChunkID] = struct{}{}
		}
		if h.ArticleID != 0 {
			perArticle[h.ArticleID]++
		}
		out = append(out, h)
	}

	for _, h := range primary {
		appendHit(h)
	}
	for _, h := range secondary {
		appendHit(h)
	}
	return out
}

// RerankHits 使用 DashScope Cross-Encoder Rerank 模型对候选片段重新打分排序
func RerankHits(question string, hits []RAGHit) ([]RAGHit, error) {
	if len(hits) == 0 {
		return hits, nil
	}
	if config.AppConfig == nil || config.AppConfig.RAG.DashScopeKey == "" {
		return nil, errors.New("dashscope key not configured")
	}

	topN := config.AppConfig.RAG.RerankTopN
	if topN <= 0 {
		topN = len(hits)
	}
	if topN > len(hits) {
		topN = len(hits)
	}

	// 提取所有文本片段
	documents := make([]string, len(hits))
	for i, h := range hits {
		documents[i] = h.Content
	}

	reqBody := map[string]interface{}{
		"model": config.AppConfig.RAG.RerankModel,
		"input": map[string]interface{}{
			"query":     question,
			"documents": documents,
		},
		"parameters": map[string]interface{}{
			"top_n":            topN,
			"return_documents": false,
		},
	}

	body, _ := json.Marshal(reqBody)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	url := "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.AppConfig.RAG.DashScopeKey)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank api failed: status=%d body=%s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Output struct {
			Results []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
			} `json:"results"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("rerank parse failed: %w, body=%s", err, string(respBytes))
	}

	// 按 relevance_score 重新排序
	reranked := make([]RAGHit, 0, len(result.Output.Results))
	for _, r := range result.Output.Results {
		if r.Index >= 0 && r.Index < len(hits) {
			h := hits[r.Index]
			score := r.RelevanceScore
			h.RerankScore = &score
			reranked = append(reranked, h)
		}
	}

	logrus.Infof("rerank: %d candidates → %d results (model=%s)", len(hits), len(reranked), config.AppConfig.RAG.RerankModel)
	return reranked, nil
}
