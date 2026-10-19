package utils

import (
	"back/config"
	"back/global"
	"back/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ==================== ES 索引操作 ====================

// IndexArticleToES 将文章写入 Elasticsearch
func IndexArticleToES(article *models.Article) error {
	if global.ESClient == nil {
		return nil
	}

	doc := map[string]interface{}{
		"article_id": article.ID,
		"title":      article.Title,
		"preview":    article.Preview,
		"content":    article.Content,
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal es doc: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := global.ESClient.Index(
		config.ArticlesIndex,
		bytes.NewReader(body),
		global.ESClient.Index.WithDocumentID(fmt.Sprintf("%d", article.ID)),
		global.ESClient.Index.WithContext(ctx),
		global.ESClient.Index.WithRefresh("true"),
	)
	if err != nil {
		logrus.Warnf("ES index article %d failed: %v", article.ID, err)
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		errBody, _ := io.ReadAll(resp.Body)
		logrus.Warnf("ES index article %d error: %s", article.ID, string(errBody))
		return fmt.Errorf("es index article %d failed: %s", article.ID, string(errBody))
	}
	return nil
}

// DeleteArticleFromES 从 ES 中删除文章
func DeleteArticleFromES(articleID uint) error {
	if global.ESClient == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := global.ESClient.Delete(
		config.ArticlesIndex,
		fmt.Sprintf("%d", articleID),
		global.ESClient.Delete.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ==================== ES BM25 全文检索 ====================

type esHit struct {
	ArticleID uint    `json:"article_id"`
	Score     float64 `json:"score"`
}

// ESBM25Search 使用 Elasticsearch 的 BM25 算法进行全文检索
// 返回 article_id 和 BM25 分数的有序列表
func ESBM25Search(question string, limit int) ([]esHit, error) {
	hits, _, err := ESBM25SearchPage(question, 0, limit)
	return hits, err
}

func ESBM25SearchPage(question string, from int, limit int) ([]esHit, int, error) {
	if global.ESClient == nil {
		return nil, 0, fmt.Errorf("ES client not initialized")
	}
	q := strings.TrimSpace(question)
	if q == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 20
	}

	query := map[string]interface{}{
		"from":             from,
		"size":             limit,
		"track_total_hits": true,
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  q,
				"fields": []string{"title^3", "preview^2", "content"},
				"type":   "best_fields",
			},
		},
		"_source": []string{"article_id"},
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal es query: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := global.ESClient.Search(
		global.ESClient.Search.WithContext(ctx),
		global.ESClient.Search.WithIndex(config.ArticlesIndex),
		global.ESClient.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("es search: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("es search error: %s", string(errBody))
	}

	var result esSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decode es response: %w", err)
	}

	hits := make([]esHit, 0, len(result.Hits.Hits))
	for _, h := range result.Hits.Hits {
		hits = append(hits, esHit{
			ArticleID: h.Source.ArticleID,
			Score:     h.Score,
		})
	}
	return hits, result.Hits.Total.Value, nil
}

type esSearchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Score  float64 `json:"_score"`
			Source struct {
				ArticleID uint `json:"article_id"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

// ==================== RRF 重排融合 ====================

const rrfK = 60.0

// RRFFusion 使用 Reciprocal Rank Fusion 算法融合向量检索和 BM25 检索结果
// vecHits:  向量检索结果（有序列表，第一个排名最高）
// esHits:   ES BM25 检索结果（有序列表，第一个排名最高）
// 返回融合后按 RRF 分数降序排列的 articleID 列表
func RRFFusion(vecHits []RAGHit, esHits []esHit) []uint {
	// articleID -> RRF 累积分数
	scores := make(map[uint]float64, len(vecHits)+len(esHits))

	// 向量检索贡献（按 RAGHit 中 articleID 去重排名）
	seen := make(map[uint]bool, len(vecHits))
	rank := 0
	for _, h := range vecHits {
		if seen[h.ArticleID] {
			continue
		}
		seen[h.ArticleID] = true
		rank++
		scores[h.ArticleID] += 1.0 / (rrfK + float64(rank))
	}

	// ES BM25 检索贡献
	for rank, h := range esHits {
		scores[h.ArticleID] += 1.0 / (rrfK + float64(rank+1))
	}

	// 按 RRF 分数降序排列
	type kv struct {
		id    uint
		score float64
	}
	kvs := make([]kv, 0, len(scores))
	for id, score := range scores {
		kvs = append(kvs, kv{id, score})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].score == kvs[j].score {
			return kvs[i].id < kvs[j].id
		}
		return kvs[i].score > kvs[j].score
	})

	ids := make([]uint, 0, len(kvs))
	for _, kv := range kvs {
		ids = append(ids, kv.id)
	}
	return ids
}

// ==================== 替换旧混合检索的函数 ====================

// RetrieveRAGHitsWithES 使用 ES BM25 + 向量检索 + RRF 融合的混合检索
// 替代旧的 RetrieveRAGHits
func RetrieveRAGHitsWithES(question string, topK int) ([]RAGHit, error) {
	if topK <= 0 {
		topK = config.AppConfig.RAG.TopK
		if topK <= 0 {
			topK = 5
		}
	}

	parentTopN := config.AppConfig.RAG.ParentTopN
	if parentTopN <= 0 {
		parentTopN = 20
	}

	// 第 1 路：向量语义检索（ChromaDB）
	vecHits, err := QueryRelevantChunks(question, topK)
	if err != nil {
		return nil, err
	}

	// 第 2 路：ES BM25 关键词检索
	var allIDs []uint
	esHits, esErr := ESBM25Search(question, parentTopN)
	if esErr != nil {
		logrus.Warnf("ES BM25 search failed, fallback to vector only: %v", esErr)
	} else if len(esHits) > 0 {
		// RRF 融合两路结果
		allIDs = RRFFusion(vecHits, esHits)

		// 用融合后的文章列表在向量库中做二次检索
		if len(allIDs) > 0 {
			// 只取前 parentTopN 篇文章做 filtered query
			filterIDs := allIDs
			if len(filterIDs) > parentTopN {
				filterIDs = filterIDs[:parentTopN]
			}
			filteredHits, fErr := QueryRelevantChunksFiltered(question, topK, filterIDs)
			if fErr == nil && len(filteredHits) > 0 {
				// RRF 已决定文章顺序，这里再按 RRF 文章顺序对片段排序
				vecHits = sortHitsByArticleOrder(filteredHits, allIDs, topK)
			}
		}
	}

	// Parent-Child 上下文附加
	if config.AppConfig != nil && config.AppConfig.RAG.ParentChildEnabled {
		vecHits = AttachParentContexts(vecHits)
	}

	// Cross-Encoder Rerank
	if config.AppConfig != nil && config.AppConfig.RAG.RerankEnabled && len(vecHits) > 1 {
		reranked, err := RerankHits(question, vecHits)
		if err != nil {
			logrus.Warnf("rerank failed, fallback to original order: %v", err)
		} else {
			return reranked, nil
		}
	}

	return vecHits, nil
}

// sortHitsByArticleOrder 按 RRF 排好的文章顺序重排片段，并限制结果数量
func sortHitsByArticleOrder(hits []RAGHit, order []uint, maxTotal int) []RAGHit {
	if maxTotal <= 0 {
		maxTotal = 5
	}

	// 构建 article 顺序映射
	rank := make(map[uint]int, len(order))
	for i, id := range order {
		rank[id] = i
	}

	seenArticle := make(map[uint]int, maxTotal)
	maxPerArticle := config.AppConfig.RAG.MaxChunksPerArticle
	if maxPerArticle <= 0 {
		maxPerArticle = 3
	}

	out := make([]RAGHit, 0, maxTotal)
	for _, h := range hits {
		if len(out) >= maxTotal {
			break
		}
		if seenArticle[h.ArticleID] >= maxPerArticle {
			continue
		}
		seenArticle[h.ArticleID]++
		out = append(out, h)
	}

	// 按 RRF 文章排名 + 原始得分排序
	sort.SliceStable(out, func(i, j int) bool {
		ri, oki := rank[out[i].ArticleID]
		rj, okj := rank[out[j].ArticleID]
		if oki && okj {
			if ri != rj {
				return ri < rj
			}
		}
		return out[i].Distance < out[j].Distance
	})

	if len(out) > maxTotal {
		out = out[:maxTotal]
	}
	return out
}

// ==================== RAGHit 辅助 ====================

// CosineToDistance 余弦相似度转伪距离（越小越好，供排序用）
func CosineToDistance(sim float64) float64 {
	return 1.0 - sim
}

// NormalizeScore 归一化分数到 [0,1] 范围
func NormalizeScore(score, min, max float64) float64 {
	if max <= min {
		return 0.5
	}
	return math.Max(0, math.Min(1, (score-min)/(max-min)))
}
