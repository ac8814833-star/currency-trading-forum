package utils

import (
	"back/config"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	openaiEmbedding "github.com/cloudwego/eino-ext/components/embedding/openai"
	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type RAGHit struct {
	ChunkID            string     `json:"chunk_id"`
	ArticleID          uint       `json:"article_id"`
	Title              string     `json:"title"`
	Content            string     `json:"-"`
	Distance           float64    `json:"distance,omitempty"`
	Source             string     `json:"source"`
	RagVersion         int        `json:"-"`
	IndexSchemaVersion int        `json:"-"`
	DocType            string     `json:"doc_type"`
	ChunkIndex         int        `json:"chunk_index"`
	ParentID           string     `json:"parent_id,omitempty"`
	ParentContent      string     `json:"-"`
	ArticleUpdatedAt   *time.Time `json:"article_updated_at,omitempty"`
	Excerpt            string     `json:"excerpt,omitempty"`
	Rank               int        `json:"rank,omitempty"`
	RerankScore        *float64   `json:"rerank_score,omitempty"`
}

type chromaQueryResult struct {
	IDs       [][]string                 `json:"ids"`
	Documents [][]string                 `json:"documents"`
	Metadatas [][]map[string]interface{} `json:"metadatas"`
	Distances [][]float64                `json:"distances"`
}

func ChunkText(text string, chunkSize int, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 400
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = 80
	}

	// 使用 Eino Recursive Splitter：按分隔符优先级递归切割，保证段落/句子完整
	splitter, err := recursive.NewSplitter(context.Background(), &recursive.Config{
		ChunkSize:   chunkSize,
		OverlapSize: overlap,
		Separators:  []string{"\n\n", "\n", "。", "！", "？", "；", "，", " ", ""},
		LenFunc:     utf8.RuneCountInString,  // 按字符数（而非字节数）计算长度
		KeepType:    recursive.KeepTypeStart, // 分隔符保留在下一段开头
	})
	if err != nil {
		// 回退到原始简单切割
		return simpleChunkText(text, chunkSize, overlap)
	}

	docs, err := splitter.Transform(context.Background(), []*schema.Document{
		{ID: "chunk", Content: text},
	})
	if err != nil {
		return simpleChunkText(text, chunkSize, overlap)
	}

	var chunks []string
	for _, doc := range docs {
		chunk := strings.TrimSpace(doc.Content)
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

// simpleChunkText 原始固定大小切割，作为回退方案
func simpleChunkText(text string, chunkSize int, overlap int) []string {
	runes := []rune(text)
	var chunks []string
	step := chunkSize - overlap
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
	}
	return chunks
}

func EmbedTexts(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, errors.New("texts is empty")
	}
	emb, err := getDashScopeEmbedder()
	if err != nil {
		return nil, err
	}
	return emb.EmbedStrings(context.Background(), texts)
}

var (
	dashScopeEmbedderOnce sync.Once
	dashScopeEmbedder     embedding.Embedder
	dashScopeEmbedderErr  error

	dashScopeChatModelOnce sync.Once
	dashScopeChatModel     model.BaseChatModel
	dashScopeChatModelErr  error
)

func getDashScopeEmbedder() (embedding.Embedder, error) {
	dashScopeEmbedderOnce.Do(func() {
		if config.AppConfig == nil {
			dashScopeEmbedderErr = errors.New("config is not initialized")
			return
		}
		if config.AppConfig.RAG.DashScopeKey == "" {
			dashScopeEmbedderErr = errors.New("rag.dashScopeKey is empty")
			return
		}
		if config.AppConfig.RAG.EmbeddingModel == "" {
			dashScopeEmbedderErr = errors.New("rag.embeddingModel is empty")
			return
		}
		if config.AppConfig.RAG.DashScopeBase == "" {
			dashScopeEmbedderErr = errors.New("rag.dashScopeBase is empty")
			return
		}

		ctx := context.Background()
		dashScopeEmbedder, dashScopeEmbedderErr = openaiEmbedding.NewEmbedder(ctx, &openaiEmbedding.EmbeddingConfig{
			APIKey:  config.AppConfig.RAG.DashScopeKey,
			BaseURL: strings.TrimRight(config.AppConfig.RAG.DashScopeBase, "/"),
			Model:   config.AppConfig.RAG.EmbeddingModel,
			Timeout: 40 * time.Second,
		})
	})
	return dashScopeEmbedder, dashScopeEmbedderErr
}

func getDashScopeChatModel() (model.BaseChatModel, error) {
	dashScopeChatModelOnce.Do(func() {
		if config.AppConfig == nil {
			dashScopeChatModelErr = errors.New("config is not initialized")
			return
		}
		if config.AppConfig.RAG.DashScopeKey == "" {
			dashScopeChatModelErr = errors.New("rag.dashScopeKey is empty")
			return
		}
		if config.AppConfig.RAG.ChatModel == "" {
			dashScopeChatModelErr = errors.New("rag.chatModel is empty")
			return
		}
		if config.AppConfig.RAG.DashScopeBase == "" {
			dashScopeChatModelErr = errors.New("rag.dashScopeBase is empty")
			return
		}

		ctx := context.Background()
		dashScopeChatModel, dashScopeChatModelErr = openaiModel.NewChatModel(ctx, &openaiModel.ChatModelConfig{
			APIKey:  config.AppConfig.RAG.DashScopeKey,
			BaseURL: strings.TrimRight(config.AppConfig.RAG.DashScopeBase, "/"),
			Model:   config.AppConfig.RAG.ChatModel,
			Timeout: 60 * time.Second,
		})
	})
	return dashScopeChatModel, dashScopeChatModelErr
}

var (
	chromaCollectionMu  sync.Mutex
	chromaCollectionKey string
	chromaCollectionID  string
)

func chromaV2BaseURL() (string, error) {
	if config.AppConfig == nil {
		return "", errors.New("config is not initialized")
	}
	base := strings.TrimRight(config.AppConfig.RAG.ChromaURL, "/")
	if base == "" {
		return "", errors.New("rag.chromaURL is empty")
	}
	return base + "/api/v2/tenants/default_tenant/databases/default_database", nil
}

func findChromaCollectionID(ctx context.Context, createIfMissing bool) (string, error) {
	base, err := chromaV2BaseURL()
	if err != nil {
		return "", err
	}
	collectionName := strings.TrimSpace(config.AppConfig.RAG.Collection)
	if collectionName == "" {
		return "", errors.New("rag.collection is empty")
	}

	key := base + "|" + collectionName

	// 锁定整个检查-创建-缓存流程，防止并发重复创建
	chromaCollectionMu.Lock()
	defer chromaCollectionMu.Unlock()

	// 快速路径：已缓存
	if chromaCollectionKey == key && chromaCollectionID != "" {
		return chromaCollectionID, nil
	}

	type collectionItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	client := &http.Client{Timeout: 20 * time.Second}

	listReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/collections", nil)
	if err != nil {
		return "", err
	}
	listResp, err := client.Do(listReq)
	if err != nil {
		return "", err
	}
	listBytes, _ := io.ReadAll(listResp.Body)
	_ = listResp.Body.Close()
	if listResp.StatusCode >= 300 {
		return "", fmt.Errorf("list collections failed: status=%d body=%s", listResp.StatusCode, string(listBytes))
	}
	var items []collectionItem
	if err := json.Unmarshal(listBytes, &items); err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Name == collectionName && it.ID != "" {
			chromaCollectionKey = key
			chromaCollectionID = it.ID
			return it.ID, nil
		}
	}
	if !createIfMissing {
		return "", nil
	}

	createBody, _ := json.Marshal(map[string]any{"name": collectionName})
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/collections", bytes.NewReader(createBody))
	if err != nil {
		return "", err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := client.Do(createReq)
	if err != nil {
		return "", err
	}
	createBytes, _ := io.ReadAll(createResp.Body)
	_ = createResp.Body.Close()

	if createResp.StatusCode >= 300 {
		return "", fmt.Errorf("create collection failed: status=%d body=%s", createResp.StatusCode, string(createBytes))
	}

	var created collectionItem
	if err := json.Unmarshal(createBytes, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", errors.New("create collection returned empty id")
	}

	chromaCollectionKey = key
	chromaCollectionID = created.ID
	return created.ID, nil
}

func getOrCreateChromaCollectionID(ctx context.Context) (string, error) {
	return findChromaCollectionID(ctx, true)
}

func getExistingChromaCollectionID(ctx context.Context) (string, error) {
	return findChromaCollectionID(ctx, false)
}

func UpsertArticleChunks(articleID uint, title string, ragVersion int, chunks []ArticleIndexChunk, vectors [][]float64) error {
	if len(chunks) == 0 || len(vectors) == 0 || len(chunks) != len(vectors) {
		return errors.New("invalid chunks or vectors")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	collectionID, err := getOrCreateChromaCollectionID(ctx)
	cancel()
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(chunks))
	metadatas := make([]map[string]interface{}, 0, len(chunks))
	documents := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		ids = append(ids, fmt.Sprintf("article_%d_v%d_s%d_%s", articleID, ragVersion, CurrentRAGIndexSchemaVersion(), chunk.IDSuffix))
		documents = append(documents, chunk.Content)
		metadatas = append(metadatas, map[string]interface{}{
			"article_id":           articleID,
			"title":                title,
			"chunk_id":             chunk.ChunkIndex,
			"doc_type":             chunk.DocType,
			"parent_id":            chunk.ParentID,
			"parent_content":       chunk.ParentContent,
			"rag_version":          ragVersion,
			"index_schema_version": CurrentRAGIndexSchemaVersion(),
		})
	}

	reqBody := map[string]interface{}{
		"ids":        ids,
		"documents":  documents,
		"embeddings": vectors,
		"metadatas":  metadatas,
	}
	body, _ := json.Marshal(reqBody)
	base, err := chromaV2BaseURL()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/collections/%s/upsert", base, collectionID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("upsert failed: %s", string(respBytes))
	}
	return nil
}

func DeleteArticleChunks(articleID uint) error {
	if articleID == 0 {
		return errors.New("article id is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	collectionID, err := getExistingChromaCollectionID(ctx)
	if err != nil {
		return err
	}
	if collectionID == "" {
		return nil
	}

	reqBody := map[string]interface{}{
		"where": map[string]interface{}{
			"article_id": articleID,
		},
	}
	body, _ := json.Marshal(reqBody)

	base, err := chromaV2BaseURL()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/collections/%s/delete", base, collectionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("delete chunks failed: %s", string(respBytes))
	}
	return nil
}

// DeleteOldArticleChunks runs only after the new version has been stored and
// marked current. A failed cleanup leaves harmless stale chunks rather than
// deleting the last usable index.
func DeleteOldArticleChunks(articleID uint, keepVersion, keepSchemaVersion int) error {
	if articleID == 0 || keepVersion <= 0 || keepSchemaVersion <= 0 {
		return errors.New("invalid article version")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	collectionID, err := getExistingChromaCollectionID(ctx)
	if err != nil || collectionID == "" {
		return err
	}
	base, err := chromaV2BaseURL()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]interface{}{
		"where": map[string]interface{}{"$and": []interface{}{
			map[string]interface{}{"article_id": articleID},
			map[string]interface{}{"$or": []interface{}{
				map[string]interface{}{"rag_version": map[string]interface{}{"$ne": keepVersion}},
				map[string]interface{}{"index_schema_version": map[string]interface{}{"$ne": keepSchemaVersion}},
			}},
		}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/collections/%s/delete", base, collectionID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete old chunks failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func buildRAGQueryWhere(articleIDs []uint) map[string]interface{} {
	// Keep old persisted chunk layouts out of the candidate pool. Without this
	// filter, stale chunks can consume Chroma's n_results before the database
	// liveness check gets a chance to discard them, starving the v2 parent-child
	// chunks during a rolling re-index.
	conditions := make([]map[string]interface{}, 0, 3)
	if len(articleIDs) > 0 {
		conditions = append(conditions, map[string]interface{}{
			"article_id": map[string]interface{}{
				"$in": articleIDs,
			},
		})
	}
	conditions = append(conditions, map[string]interface{}{
		"index_schema_version": CurrentRAGIndexSchemaVersion(),
	})
	if config.AppConfig != nil && config.AppConfig.RAG.ParentChildEnabled {
		conditions = append(conditions, map[string]interface{}{
			"doc_type": "child",
		})
	}

	switch len(conditions) {
	case 0:
		return nil
	case 1:
		return conditions[0]
	default:
		items := make([]interface{}, 0, len(conditions))
		for _, item := range conditions {
			items = append(items, item)
		}
		return map[string]interface{}{
			"$and": items,
		}
	}
}

func parseRAGHits(parsed chromaQueryResult) []RAGHit {
	if len(parsed.Documents) == 0 {
		return []RAGHit{}
	}

	hits := make([]RAGHit, 0, len(parsed.Documents[0]))
	for i, doc := range parsed.Documents[0] {
		h := RAGHit{
			Content:    doc,
			ChunkIndex: -1,
			Source:     "vector",
		}
		if len(parsed.IDs) > 0 && len(parsed.IDs[0]) > i {
			h.ChunkID = parsed.IDs[0][i]
		}
		if len(parsed.Distances) > 0 && len(parsed.Distances[0]) > i {
			h.Distance = parsed.Distances[0][i]
		}
		if len(parsed.Metadatas) > 0 && len(parsed.Metadatas[0]) > i {
			m := parsed.Metadatas[0][i]
			if v, ok := m["title"].(string); ok {
				h.Title = v
			}
			if v, ok := m["parent_content"].(string); ok {
				h.ParentContent = v
			}
			if v, ok := m["parent_id"].(string); ok {
				h.ParentID = v
			}
			if v, ok := m["doc_type"].(string); ok {
				h.DocType = v
			}
			if v, ok := m["article_id"].(float64); ok {
				h.ArticleID = uint(v)
			}
			if v, ok := m["chunk_id"].(float64); ok {
				h.ChunkIndex = int(v)
			}
			if v, ok := m["rag_version"].(float64); ok {
				h.RagVersion = int(v)
			}
			if v, ok := m["index_schema_version"].(float64); ok {
				h.IndexSchemaVersion = int(v)
			}
		}
		hits = append(hits, h)
	}
	return hits
}

func QueryRelevantChunks(question string, topK int) ([]RAGHit, error) {
	vecs, err := EmbedTexts([]string{question})
	if err != nil {
		return nil, err
	}
	return queryRelevantChunksByVectors(vecs, topK, nil)
}

func QueryRelevantChunksFiltered(question string, topK int, articleIDs []uint) ([]RAGHit, error) {
	vecs, err := EmbedTexts([]string{question})
	if err != nil {
		return nil, err
	}
	return queryRelevantChunksByVectors(vecs, topK, articleIDs)
}

func queryRelevantChunksByVectors(vecs [][]float64, topK int, articleIDs []uint) ([]RAGHit, error) {
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, errors.New("query vector is empty")
	}
	if topK <= 0 {
		topK = 5
		if config.AppConfig != nil && config.AppConfig.RAG.TopK > 0 {
			topK = config.AppConfig.RAG.TopK
		}
	}
	reqBody := map[string]interface{}{
		"query_embeddings": vecs,
		"n_results":        topK,
		"include":          []string{"documents", "metadatas", "distances"},
	}
	if where := buildRAGQueryWhere(articleIDs); where != nil {
		reqBody["where"] = where
	}
	body, _ := json.Marshal(reqBody)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	collectionID, err := getOrCreateChromaCollectionID(ctx)
	cancel()
	if err != nil {
		return nil, err
	}
	base, err := chromaV2BaseURL()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/collections/%s/query", base, collectionID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("query failed: %s", string(respBytes))
	}

	var parsed chromaQueryResult
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Documents) == 0 {
		return []RAGHit{}, nil
	}

	return parseRAGHits(parsed), nil
}

func GenerateRAGAnswer(question string, hits []RAGHit) (string, error) {
	if len(hits) == 0 {
		// 没有可信上下文时不请求模型，既避免无依据生成，也避免无效消耗 Token。
		return noAnswerRAGResponse, nil
	}

	contextText, citedHits := buildRAGPromptContext(hits)
	if contextText == "" {
		return noAnswerRAGResponse, nil
	}
	systemPrompt := buildRAGSystemPrompt(question)

	chatModel, err := getDashScopeChatModel()
	if err != nil {
		return "", err
	}

	type chainInput struct {
		Question string
		Context  string
	}

	chain := compose.NewChain[chainInput, *schema.Message]()
	chain.AppendLambda(compose.InvokableLambda(func(ctx context.Context, input chainInput) ([]*schema.Message, error) {
		return []*schema.Message{
			schema.SystemMessage(systemPrompt),
			schema.UserMessage(fmt.Sprintf("用户问题：%s\n\n检索片段：\n%s", input.Question, input.Context)),
		}, nil
	}))
	chain.AppendChatModel(chatModel)
	runnable, err := chain.Compile(context.Background())
	if err != nil {
		return "", err
	}

	out, err := runnable.Invoke(context.Background(), chainInput{
		Question: question,
		Context:  contextText,
	})
	if err != nil {
		return "", err
	}
	return normalizeRAGCitations(out.Content, citedHits), nil
}
