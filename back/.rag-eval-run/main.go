package main

import (
	"back/config"
	"back/global"
	"back/models"
	"back/utils"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func main() {
	dbName := os.Getenv("RAG_EVAL_DB")
	collection := os.Getenv("RAG_EVAL_COLLECTION")
	output := os.Getenv("RAG_EVAL_OUTPUT")
	if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(dbName) ||
		!regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(collection) || output == "" {
		fatal("invalid isolated evaluation settings")
	}

	viper.SetConfigFile("config/config.yml")
	if err := viper.ReadInConfig(); err != nil {
		fatal("read config: %v", err)
	}
	oldDSN := viper.GetString("database.dsn")
	dsnLocation := regexp.MustCompile(`@tcp\([^)]+\)/[^?]+`)
	if !dsnLocation.MatchString(oldDSN) {
		fatal("unexpected database DSN shape")
	}
	viper.Set("database.dsn", dsnLocation.ReplaceAllString(oldDSN, "@tcp(127.0.0.1:3307)/"+dbName))
	viper.Set("redis.addr", "")
	viper.Set("rabbitmq.url", "")
	viper.Set("rag.collection", collection)
	viper.Set("rag.chatModel", "qwen3.7-max")
	viper.Set("rag.embeddingModel", "qwen3.7-text-embedding")
	viper.Set("rag.elasticsearchURL", "http://localhost:9200")
	viper.Set("rag.chromaURL", "http://localhost:8000")
	viper.Set("rag.bm25Enabled", true)
	viper.Set("rag.rerankEnabled", false)
	config.InitConfig()
	defer config.CloseRabbitMQ()

	var activeDB string
	if err := global.Db.Raw("SELECT DATABASE()").Scan(&activeDB).Error; err != nil || activeDB != dbName {
		fatal("isolated DB check failed: got %q, error %v", activeDB, err)
	}
	if !global.Db.Migrator().HasColumn(&models.Article{}, "rag_index_schema_version") {
		fatal("RAG index schema version column is missing")
	}
	if config.AppConfig.RAG.ChatModel != "qwen3.7-max" || config.AppConfig.RAG.EmbeddingModel != "qwen3.7-text-embedding" {
		fatal("model override did not take effect")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	for _, id := range []uint{7, 8, 9, 10, 11} {
		var article models.Article
		if err := global.Db.First(&article, id).Error; err != nil {
			fatal("article %d: %v", id, err)
		}
		hash := utils.BuildArticleIndexHash(article)
		if article.RagStatus != utils.RAGStatusIndexed || article.ContentHash != hash || article.RagIndexSchemaVersion != utils.CurrentRAGIndexSchemaVersion() {
			utils.MarkArticleRAGPending(id, article.RagVersion, hash)
		}
		if err := utils.IndexArticleByID(ctx, id); err != nil {
			fatal("index article %d: %v", id, err)
		}
		if err := global.Db.First(&article, id).Error; err != nil || article.RagStatus != utils.RAGStatusIndexed {
			fatal("article %d index status %q: %v", id, article.RagStatus, err)
		}
		fmt.Printf("indexed article %d\n", id)
	}

	for i := 0; i < 30 && global.ESClient == nil; i++ {
		time.Sleep(time.Second)
	}
	if global.ESClient == nil {
		fatal("Elasticsearch client is unavailable")
	}
	esHits, err := utils.ESBM25Search("Circle USDC 稳定币", 10)
	if err != nil {
		fatal("Elasticsearch BM25 search: %v", err)
	}
	fmt.Printf("ES BM25 probe: %d hits\n", len(esHits))

	suite, err := utils.LoadRAGEvalSuite("testdata/rag_eval_cases.json")
	if err != nil {
		fatal("load suite: %v", err)
	}
	if v := strings.TrimSpace(os.Getenv("RAG_EVAL_LIMIT")); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 || limit > len(suite.Cases) {
			fatal("invalid evaluation limit")
		}
		suite.Cases = suite.Cases[:limit]
	}
	if os.Getenv("RAG_EVAL_RETRIEVAL_ONLY") == "1" {
		runRetrievalEvaluation(suite, output)
		return
	}
	caseIndex := 0
	runner := func(ctx context.Context, question string) (string, []utils.RAGHit, time.Duration, time.Duration, error) {
		caseIndex++
		answer, hits, retrieval, generation, err := utils.LiveRAGEvalRunner(ctx, question)
		fmt.Printf("case %02d/%02d %s retrieval=%dms generation=%dms error=%t\n", caseIndex, len(suite.Cases), suite.Cases[caseIndex-1].ID, retrieval.Milliseconds(), generation.Milliseconds(), err != nil)
		return answer, hits, retrieval, generation, err
	}
	report := utils.RunRAGEvaluation(ctx, suite, utils.RAGEvalPricing{}, runner)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal("encode report: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		fatal("create report directory: %v", err)
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		fatal("write report: %v", err)
	}
	fmt.Printf("summary: %+v\nreport: %s\n", report.Summary, output)
}

type retrievalResult struct {
	ID                 string   `json:"id"`
	ExpectedArticleIDs []uint   `json:"expected_article_ids,omitempty"`
	ExpectAnswerable   bool     `json:"expect_answerable"`
	HitArticleIDs      []uint   `json:"hit_article_ids,omitempty"`
	HitSources         []string `json:"hit_sources,omitempty"`
	ESArticleIDs       []uint   `json:"es_article_ids,omitempty"`
	ExpectedRank       int      `json:"expected_rank"`
	LatencyMS          int64    `json:"latency_ms"`
	Error              string   `json:"error,omitempty"`
}

func runRetrievalEvaluation(suite utils.RAGEvalSuite, output string) {
	results := make([]retrievalResult, 0, len(suite.Cases))
	answerable, hit1, hit3, hit5, errors := 0, 0, 0, 0, 0
	var reciprocalRank float64
	latencies := make([]int64, 0, len(suite.Cases))
	for i, item := range suite.Cases {
		entry := retrievalResult{ID: item.ID, ExpectedArticleIDs: item.ExpectedArticleIDs, ExpectAnswerable: item.ExpectAnswerable}
		esHits, esErr := utils.ESBM25Search(item.Question, 20)
		if esErr != nil {
			entry.Error = "ES: " + esErr.Error()
		} else {
			for _, h := range esHits {
				entry.ESArticleIDs = append(entry.ESArticleIDs, h.ArticleID)
			}
		}
		started := time.Now()
		hits, err := utils.RetrieveRAGHits(item.Question, 0)
		entry.LatencyMS = time.Since(started).Milliseconds()
		latencies = append(latencies, entry.LatencyMS)
		if err != nil {
			if entry.Error != "" {
				entry.Error += "; "
			}
			entry.Error += "RAG: " + err.Error()
		}
		seen := make(map[uint]bool)
		for _, hit := range hits {
			if !seen[hit.ArticleID] {
				seen[hit.ArticleID] = true
				entry.HitArticleIDs = append(entry.HitArticleIDs, hit.ArticleID)
				entry.HitSources = append(entry.HitSources, hit.Source)
			}
		}
		if entry.Error != "" {
			errors++
		}
		if item.ExpectAnswerable {
			answerable++
			for rank, id := range entry.HitArticleIDs {
				for _, expected := range item.ExpectedArticleIDs {
					if id == expected {
						entry.ExpectedRank = rank + 1
						break
					}
				}
				if entry.ExpectedRank > 0 {
					break
				}
			}
			if entry.ExpectedRank > 0 {
				reciprocalRank += 1.0 / float64(entry.ExpectedRank)
				if entry.ExpectedRank <= 1 { hit1++ }
				if entry.ExpectedRank <= 3 { hit3++ }
				if entry.ExpectedRank <= 5 { hit5++ }
			}
		}
		results = append(results, entry)
		fmt.Printf("retrieval %02d/%02d %s rank=%d hits=%v latency=%dms error=%t\n", i+1, len(suite.Cases), item.ID, entry.ExpectedRank, entry.HitArticleIDs, entry.LatencyMS, entry.Error != "")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(p float64) int64 {
		if len(latencies) == 0 { return 0 }
		idx := int(float64(len(latencies)-1)*p + 0.5)
		return latencies[idx]
	}
	summary := map[string]any{
		"total_cases": len(suite.Cases), "answerable_cases": answerable, "errors": errors,
		"hit_at_1": hit1, "hit_at_3": hit3, "hit_at_5": hit5,
		"mrr": reciprocalRank/float64(answerable),
		"p50_retrieval_ms": percentile(0.5), "p95_retrieval_ms": percentile(0.95),
	}
	data, err := json.MarshalIndent(map[string]any{"summary": summary, "results": results}, "", "  ")
	if err != nil { fatal("encode retrieval report: %v", err) }
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil { fatal("create report directory: %v", err) }
	if err := os.WriteFile(output, data, 0o644); err != nil { fatal("write retrieval report: %v", err) }
	fmt.Printf("retrieval summary: %+v\nreport: %s\n", summary, output)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
