package service

import (
	"back/models"
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ============================================================
// 1. 定义接口 — 这是「可测试」的关键
// 测试时用 Mock 实现，生产时用真实实现
// ============================================================

// ArticleRepository 数据访问接口
type ArticleRepository interface {
	FindAll(ctx context.Context) ([]models.Article, error)
	FindByID(ctx context.Context, id string) (*models.Article, error)
	Create(ctx context.Context, article *models.Article) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	Delete(ctx context.Context, id string) error
}

// ArticleCache 缓存接口
type ArticleCache interface {
	GetArticles(ctx context.Context) ([]models.Article, error)
	SetArticles(ctx context.Context, articles []models.Article, ttl time.Duration) error
	GetArticle(ctx context.Context, id string) (*models.Article, error)
	SetArticle(ctx context.Context, article *models.Article, ttl time.Duration) error
	DelArticle(ctx context.Context, id string) error
	DelArticlesList(ctx context.Context) error
}

// ============================================================
// 2. Service 持有接口，不持有具体实现
// ============================================================

type ArticleService struct {
	repo  ArticleRepository
	cache ArticleCache
}

// NewArticleService 构造函数 —— 依赖注入
func NewArticleService(repo ArticleRepository, cache ArticleCache) *ArticleService {
	return &ArticleService{repo: repo, cache: cache}
}

// ============================================================
// 3. 业务逻辑 —— 不碰任何全局变量
// ============================================================

func (s *ArticleService) GetArticles(ctx context.Context) ([]models.Article, error) {
	// 先查缓存
	articles, err := s.cache.GetArticles(ctx)
	if err == nil {
		return articles, nil
	}

	// 缓存 miss → 查数据库
	articles, err = s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// 异步写缓存
	go func() {
		_ = s.cache.SetArticles(context.Background(), articles, 10*time.Minute)
	}()

	return articles, nil
}

func (s *ArticleService) GetArticleByID(ctx context.Context, id string) (*models.Article, error) {
	article, err := s.cache.GetArticle(ctx, id)
	if err == nil {
		return article, nil
	}

	article, err = s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	go func() {
		_ = s.cache.SetArticle(context.Background(), article, time.Hour)
	}()

	return article, nil
}

// ============================================================
// 4. 定义错误 —— 让调用方可以精准判断
// ============================================================

var (
	ErrNotFound      = errors.New("article not found")
	ErrCacheMiss     = errors.New("cache miss") // 内部使用，不暴露
)

// JSON 序列化辅助（避免在 service 层重复写）
func marshalArticles(articles []models.Article) (string, error) {
	data, err := json.Marshal(articles)
	return string(data), err
}

func unmarshalArticles(data string) ([]models.Article, error) {
	var articles []models.Article
	err := json.Unmarshal([]byte(data), &articles)
	return articles, err
}
