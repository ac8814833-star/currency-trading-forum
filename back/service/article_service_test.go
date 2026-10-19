package service

import (
	"back/models"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ============================================================
// Mock 实现 — 只在测试中用，模拟数据库和缓存
// ============================================================

// mockArticleRepo 假装是数据库
type mockArticleRepo struct {
	articles map[string]*models.Article // 用 map 当"数据库"
}

func newMockRepo() *mockArticleRepo {
	return &mockArticleRepo{articles: make(map[string]*models.Article)}
}

func (m *mockArticleRepo) FindAll(ctx context.Context) ([]models.Article, error) {
	var result []models.Article
	for _, a := range m.articles {
		result = append(result, *a)
	}
	return result, nil
}

func (m *mockArticleRepo) FindByID(ctx context.Context, id string) (*models.Article, error) {
	a, ok := m.articles[id]
	if !ok {
		return nil, errors.New("record not found")
	}
	return a, nil
}

func (m *mockArticleRepo) Create(ctx context.Context, article *models.Article) error {
	m.articles[article.Title] = article
	return nil
}

func (m *mockArticleRepo) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}

func (m *mockArticleRepo) Delete(ctx context.Context, id string) error {
	return nil
}

// mockArticleCache 假装是 Redis
type mockArticleCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMockCache() *mockArticleCache {
	return &mockArticleCache{data: make(map[string]string)}
}

func (m *mockArticleCache) GetArticles(ctx context.Context) ([]models.Article, error) {
	m.mu.RLock()
	data, ok := m.data["articles_list"]
	m.mu.RUnlock()
	if !ok {
		return nil, errors.New("cache miss")
	}
	return unmarshalArticles(data)
}

func (m *mockArticleCache) SetArticles(ctx context.Context, articles []models.Article, ttl time.Duration) error {
	data, err := marshalArticles(articles)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.data["articles_list"] = data
	m.mu.Unlock()
	return nil
}

func (m *mockArticleCache) GetArticle(ctx context.Context, id string) (*models.Article, error) {
	m.mu.RLock()
	data, ok := m.data["article:"+id]
	m.mu.RUnlock()
	if !ok {
		return nil, errors.New("cache miss")
	}
	var article models.Article
	if err := unmarshalSingle(data, &article); err != nil {
		return nil, err
	}
	return &article, nil
}

func (m *mockArticleCache) SetArticle(ctx context.Context, article *models.Article, ttl time.Duration) error {
	// 简化实现
	return nil
}

func (m *mockArticleCache) DelArticle(ctx context.Context, id string) error {
	m.mu.Lock()
	delete(m.data, "article:"+id)
	m.mu.Unlock()
	return nil
}

func (m *mockArticleCache) DelArticlesList(ctx context.Context) error {
	m.mu.Lock()
	delete(m.data, "articles_list")
	m.mu.Unlock()
	return nil
}

func unmarshalSingle(data string, v interface{}) error {
	// 简化：实际项目用 json.Unmarshal
	return nil
}

// ============================================================
// 测试！不连数据库、不连 Redis
// ============================================================

func TestArticleService_GetArticles_FromDB(t *testing.T) {
	// Arrange：准备 mock 依赖
	repo := newMockRepo()
	cache := newMockCache()
	svc := NewArticleService(repo, cache) // 注入 mock！

	// 往 mock 数据库里放一些数据
	repo.articles["1"] = &models.Article{Title: "Go 测试入门"}
	repo.articles["2"] = &models.Article{Title: "接口与 Mock"}

	// Act：调用 service（缓存是空的，会穿透到 DB）
	ctx := context.Background()
	articles, err := svc.GetArticles(ctx)

	// Assert：验证结果
	if err != nil {
		t.Fatalf("GetArticles() unexpected error: %v", err)
	}
	if len(articles) != 2 {
		t.Errorf("GetArticles() returned %d articles, want 2", len(articles))
	}
}

func TestArticleService_GetArticles_FromCache(t *testing.T) {
	// Arrange：缓存中有数据，DB 是空的
	repo := newMockRepo() // 空的
	cache := newMockCache()
	svc := NewArticleService(repo, cache)

	// 先写缓存
	cached := []models.Article{
		{Title: "缓存里的文章"},
	}
	_ = cache.SetArticles(context.Background(), cached, time.Minute)

	// Act
	articles, err := svc.GetArticles(context.Background())

	// Assert：应该从缓存返回，不碰 DB
	if err != nil {
		t.Fatalf("GetArticles() error: %v", err)
	}
	if len(articles) != 1 {
		t.Errorf("expected 1 article from cache, got %d", len(articles))
	}
	if articles[0].Title != "缓存里的文章" {
		t.Errorf("unexpected title: %s", articles[0].Title)
	}
}

func TestArticleService_GetArticles_EmptyDB(t *testing.T) {
	// 边界条件：数据库和缓存都是空的
	repo := newMockRepo()
	cache := newMockCache()
	svc := NewArticleService(repo, cache)

	articles, err := svc.GetArticles(context.Background())

	if err != nil {
		t.Fatalf("GetArticles() error: %v", err)
	}
	if len(articles) != 0 {
		t.Errorf("expected 0 articles, got %d", len(articles))
	}
}

// ============================================================
// 错误场景测试 — 面试最加分
// ============================================================

// errorRepo 故意返回错误，测异常路径
type errorRepo struct{}

func (e *errorRepo) FindAll(ctx context.Context) ([]models.Article, error) {
	return nil, errors.New("database connection refused")
}
func (e *errorRepo) FindByID(ctx context.Context, id string) (*models.Article, error) {
	return nil, errors.New("database connection refused")
}
func (e *errorRepo) Create(ctx context.Context, article *models.Article) error {
	return errors.New("database connection refused")
}
func (e *errorRepo) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return errors.New("database connection refused")
}
func (e *errorRepo) Delete(ctx context.Context, id string) error {
	return errors.New("database connection refused")
}

func TestArticleService_GetArticles_DBError(t *testing.T) {
	// 缓存 miss + 数据库炸了 → 应该返回错误
	svc := NewArticleService(&errorRepo{}, newMockCache())

	_, err := svc.GetArticles(context.Background())

	if err == nil {
		t.Fatal("GetArticles() should return error when DB fails")
	}
	t.Logf("Got expected error: %v", err)
}

// ============================================================
// 并发安全测试 — 大厂必问
// ============================================================

func TestArticleService_GetArticles_Concurrent(t *testing.T) {
	repo := newMockRepo()
	repo.articles["1"] = &models.Article{Title: "并发测试"}
	cache := newMockCache()
	svc := NewArticleService(repo, cache)

	// 同时启动 10 个 goroutine 调用
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := svc.GetArticles(context.Background())
			if err != nil {
				t.Errorf("concurrent GetArticles() error: %v", err)
			}
			done <- true
		}()
	}

	// 等所有完成
	for i := 0; i < 10; i++ {
		<-done
	}
}
