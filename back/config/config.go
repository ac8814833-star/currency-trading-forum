package config

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type config struct {
	App struct {
		Name string
		Port string
	}
	Database struct {
		DSN          string
		MaxIdleConns int
		MaxOpenConns int
	}
	Redis struct {
		Addr     string
		Password string
		DB       int
	}
	RabbitMQ struct {
		URL               string
		QueueName         string
		RAGIndexQueueName string
		LikeWorkerCount   int
	}
	JWT struct {
		Secret              string
		AccessExpireMinutes int
		RefreshExpireDays   int
	}
	RAG struct {
		ChromaURL             string
		ElasticsearchURL      string
		Collection            string
		DashScopeKey          string
		DashScopeBase         string
		ChatModel             string
		EmbeddingModel        string
		RerankModel           string
		TopK                  int
		ChunkSize             int
		ChunkOverlap          int
		ParentChunkSize       int
		ParentChunkOverlap    int
		IndexSchemaVersion    int
		ShortArticleThreshold int
		AutoIndex             bool
		SyncIntervalSecond    int
		SyncBatchSize         int
		FullSyncOnStart       bool
		BM25Enabled           bool
		ParentChildEnabled    bool
		RerankEnabled         bool
		ParentTopN            int
		MaxChunksPerArticle   int
		RerankTopN            int
	}
}

var AppConfig *config

func InitConfig() {
	InitLogger() // 初始化日志
	// 本地开发时允许从项目根目录 .env 读取密钥；容器/生产环境优先使用
	// 已注入的系统环境变量，避免把密钥写进 config.yml。
	loadDotEnv(filepath.Join("..", ".env"))
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath("./config")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file, %v", err)
	}
	AppConfig = &config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		log.Fatalf("unable to decode config into struct, %v", err)
	}
	if v := strings.TrimSpace(os.Getenv("DATABASE_DSN")); v != "" {
		AppConfig.Database.DSN = v
	}
	if v := strings.TrimSpace(os.Getenv("RAG_DASHSCOPE_KEY")); v != "" {
		AppConfig.RAG.DashScopeKey = v
	}
	if v := strings.TrimSpace(os.Getenv("JWT_SECRET")); v != "" {
		AppConfig.JWT.Secret = v
	}
	if v := strings.TrimSpace(os.Getenv("RAG_CHROMA_URL")); v != "" {
		AppConfig.RAG.ChromaURL = v
	}
	if v := strings.TrimSpace(os.Getenv("RAG_ELASTICSEARCH_URL")); v != "" {
		AppConfig.RAG.ElasticsearchURL = v
	}
	if v := strings.TrimSpace(os.Getenv("RAG_COLLECTION")); v != "" {
		AppConfig.RAG.Collection = v
	}
	if v := strings.TrimSpace(os.Getenv("RAG_TOPK")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.TopK = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_CHUNK_SIZE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.ChunkSize = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_CHUNK_OVERLAP")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.ChunkOverlap = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_PARENT_CHUNK_SIZE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.ParentChunkSize = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_PARENT_CHUNK_OVERLAP")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.ParentChunkOverlap = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_INDEX_SCHEMA_VERSION")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.IndexSchemaVersion = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_SHORT_ARTICLE_THRESHOLD")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.ShortArticleThreshold = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_AUTO_INDEX")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			AppConfig.RAG.AutoIndex = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_FULL_SYNC_ON_START")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			AppConfig.RAG.FullSyncOnStart = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_SYNC_INTERVAL_SECOND")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.SyncIntervalSecond = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_SYNC_BATCH_SIZE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.SyncBatchSize = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_BM25_ENABLED")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			AppConfig.RAG.BM25Enabled = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_PARENT_CHILD_ENABLED")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			AppConfig.RAG.ParentChildEnabled = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_RERANK_ENABLED")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			AppConfig.RAG.RerankEnabled = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_PARENT_TOPN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.ParentTopN = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_MAX_CHUNKS_PER_ARTICLE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.MaxChunksPerArticle = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_RERANK_TOPN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			AppConfig.RAG.RerankTopN = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("LIKE_WORKER_COUNT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			AppConfig.RabbitMQ.LikeWorkerCount = n
		}
	}
	if AppConfig.RAG.RerankModel == "" {
		AppConfig.RAG.RerankModel = "gte-rerank"
	}
	if AppConfig.RabbitMQ.LikeWorkerCount <= 0 {
		AppConfig.RabbitMQ.LikeWorkerCount = 4
	}
	if AppConfig.RAG.SyncIntervalSecond == 0 {
		AppConfig.RAG.SyncIntervalSecond = 300
	}
	if AppConfig.RAG.SyncBatchSize == 0 {
		AppConfig.RAG.SyncBatchSize = 50
	}
	if AppConfig.RAG.ChunkSize == 0 {
		AppConfig.RAG.ChunkSize = 400
	}
	if AppConfig.RAG.ChunkOverlap == 0 {
		AppConfig.RAG.ChunkOverlap = 80
	}
	if AppConfig.RAG.ParentChunkSize == 0 {
		AppConfig.RAG.ParentChunkSize = 1200
	}
	if AppConfig.RAG.ParentChunkOverlap == 0 {
		AppConfig.RAG.ParentChunkOverlap = 160
	}
	if AppConfig.RAG.IndexSchemaVersion == 0 {
		// Bump this whenever the persisted chunk layout or metadata contract changes.
		AppConfig.RAG.IndexSchemaVersion = 2
	}
	if AppConfig.RAG.ShortArticleThreshold == 0 {
		AppConfig.RAG.ShortArticleThreshold = 800
	}
	if AppConfig.RAG.ParentTopN == 0 {
		AppConfig.RAG.ParentTopN = 20
	}
	if AppConfig.RAG.MaxChunksPerArticle == 0 {
		AppConfig.RAG.MaxChunksPerArticle = 3
	}
	if AppConfig.RAG.RerankTopN == 0 {
		AppConfig.RAG.RerankTopN = 5
	}
	initDB()
	InitRedis()
	InitRabbitMQ()
}

// loadDotEnv 是一个最小化的 .env 读取器，仅用于本地开发。环境中已有的
// 变量不覆盖，部署环境仍通过 Docker/Kubernetes 等注入密钥。
func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
