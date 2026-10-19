package controllers_test

import (
	"back/controllers"
	"back/global"
	"back/models"
	"back/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Set TEST_MYSQL_ADMIN_DSN to run this against a local disposable MySQL server.
// The test creates and drops only its own uniquely named schema.
func TestCorrectnessIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_ADMIN_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_ADMIN_DSN for MySQL integration test")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("forum_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(schema, "forum_test_") {
		t.Fatal("unsafe schema name")
	}
	if err := admin.Exec("CREATE DATABASE `" + schema + "`").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE `" + schema + "`").Error; err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
	})
	parsed.DBName = schema
	parsed.ParseTime = true
	db, err := gorm.Open(mysql.Open(parsed.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Article{}, &models.Comment{}, &models.Like{}, &models.ExchangeRate{}); err != nil {
		t.Fatal(err)
	}
	previousDB := global.Db
	global.Db = db
	t.Cleanup(func() { global.Db = previousDB })
	user := models.User{Username: "audit", Password: "unused"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	a := models.Article{Title: "A", Preview: "A preview", Content: "A content", UserID: user.ID, Username: user.Username}
	b := models.Article{Title: "B", Preview: "B preview", Content: "B content", UserID: user.ID, Username: user.Username}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", user.ID); c.Set("username", user.Username); c.Next() })
	r.POST("/articles/:id/like", controllers.LikeArticle)
	r.POST("/articles/:id/unlike", controllers.DisLikeArticle)
	r.GET("/articles/:id/like", controllers.GetArticleLikes)
	r.GET("/articles/hot", controllers.GetHotArticles)
	r.GET("/articles", controllers.GetArticles)
	r.PUT("/articles/:id", controllers.UpdateArticle)
	r.GET("/captcha", controllers.GetCaptcha)
	r.POST("/register", controllers.Register)
	r.POST("/articles/:id/comments", controllers.CreateComment)
	r.GET("/articles/:id/comments", controllers.GetComments)
	r.GET("/articles/:id/comments/count", controllers.GetCommentCount)
	r.DELETE("/comments/:id", controllers.DeleteComment)
	r.GET("/currency/rates", controllers.GetCurrencyRates)
	r.GET("/currency/convert", controllers.ConvertCurrency)

	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		r.ServeHTTP(w, req)
		return w
	}
	assertStatus := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
	}

	articlePath := fmt.Sprintf("/articles/%d", a.ID)
	assertStatus(call(http.MethodGet, "/captcha", ""), http.StatusServiceUnavailable)
	assertStatus(call(http.MethodPost, "/register", `{"username":"no_redis_new_user","password":"secret"}`), http.StatusServiceUnavailable)
	var rolledBackUserCount int64
	if err := db.Model(&models.User{}).Where("username = ?", "no_redis_new_user").Count(&rolledBackUserCount).Error; err != nil || rolledBackUserCount != 0 {
		t.Fatalf("failed registration left a user behind: count=%d error=%v", rolledBackUserCount, err)
	}
	assertStatus(call(http.MethodPut, articlePath, `{"title":"Updated without Redis"}`), http.StatusOK)
	if err := db.First(&a, a.ID).Error; err != nil || a.Title != "Updated without Redis" {
		t.Fatalf("article edit without Redis: title=%q error=%v", a.Title, err)
	}
	assertStatus(call(http.MethodPost, articlePath+"/like", ""), 200)
	assertStatus(call(http.MethodPost, articlePath+"/like", ""), 400)
	assertStatus(call(http.MethodPost, "/articles/999999/like", ""), 404)
	assertStatus(call(http.MethodPost, articlePath+"/unlike", ""), 200)
	assertStatus(call(http.MethodPost, articlePath+"/like", ""), 200)
	likeResponse := call(http.MethodGet, articlePath+"/like", "")
	assertStatus(likeResponse, 200)
	var likeState struct {
		Likes   int64 `json:"likes"`
		IsLiked bool  `json:"is_liked"`
	}
	if err := json.Unmarshal(likeResponse.Body.Bytes(), &likeState); err != nil {
		t.Fatal(err)
	}
	if likeState.Likes != 1 || !likeState.IsLiked {
		t.Fatalf("wrong like state: %+v", likeState)
	}
	// A configured Redis outage is not the same as DB-only mode: accepted
	// outbox events could make the database projection stale.
	disconnected := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	global.RedisDB = disconnected
	t.Cleanup(func() {
		global.RedisDB = nil
		_ = disconnected.Close()
	})
	assertStatus(call(http.MethodPost, articlePath+"/unlike", ""), http.StatusServiceUnavailable)
	assertStatus(call(http.MethodGet, articlePath+"/like", ""), http.StatusServiceUnavailable)
	assertStatus(call(http.MethodGet, "/articles/hot", ""), http.StatusServiceUnavailable)
	global.RedisDB = nil
	var persistedLikes int64
	if err := db.Model(&models.Like{}).Where("article_id = ? AND user_id = ?", a.ID, user.ID).Count(&persistedLikes).Error; err != nil || persistedLikes != 1 {
		t.Fatalf("Redis outage changed MySQL like state: count=%d error=%v", persistedLikes, err)
	}
	assertStatus(call(http.MethodGet, "/articles/hot", ""), 200)
	listResponse := call(http.MethodGet, "/articles?page=1&pageSize=1", "")
	assertStatus(listResponse, 200)
	var list struct {
		Data  []models.Article `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Total != 2 {
		t.Fatalf("article pagination failed: %+v", list)
	}

	commentPath := fmt.Sprintf("/articles/%d/comments", a.ID)
	wrongID := fmt.Sprintf(`{"article_id":%d,"content":"root"}`, b.ID)
	rootResponse := call(http.MethodPost, commentPath, wrongID)
	assertStatus(rootResponse, 201)
	var root models.Comment
	if err := json.Unmarshal(rootResponse.Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	if root.ArticleID != a.ID {
		t.Fatalf("body article_id overrode URL: %d", root.ArticleID)
	}
	replyResponse := call(http.MethodPost, commentPath, fmt.Sprintf(`{"content":"reply","parent_id":%d}`, root.ID))
	assertStatus(replyResponse, 201)
	assertStatus(call(http.MethodPost, commentPath, `{"content":"another root"}`), 201)
	pageResponse := call(http.MethodGet, commentPath+"?page=1&pageSize=1", "")
	assertStatus(pageResponse, 200)
	var page struct {
		Data  []models.Comment `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Total != 2 {
		t.Fatalf("pagination failed: %+v", page)
	}
	assertStatus(call(http.MethodDelete, fmt.Sprintf("/comments/%d", root.ID), ""), 200)
	countResponse := call(http.MethodGet, commentPath+"/count", "")
	assertStatus(countResponse, 200)
	var count struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(countResponse.Body.Bytes(), &count); err != nil {
		t.Fatal(err)
	}
	if count.Count != 1 {
		t.Fatalf("orphan reply counted: %d", count.Count)
	}

	now := time.Now()
	for _, rate := range []models.ExchangeRate{
		{FromCurrency: "USD", ToCurrency: "CNY", Rate: 7.25, Date: now.Add(-time.Hour)},
		{FromCurrency: "USD", ToCurrency: "CNY", Rate: 8, Date: now},
	} {
		if err := db.Create(&rate).Error; err != nil {
			t.Fatal(err)
		}
	}
	convertResponse := call(http.MethodGet, "/currency/convert?from=usd&to=cny&amount=100", "")
	assertStatus(convertResponse, 200)
	var converted struct {
		Converted string `json:"converted"`
	}
	if err := json.Unmarshal(convertResponse.Body.Bytes(), &converted); err != nil {
		t.Fatal(err)
	}
	if converted.Converted != "800.00" {
		t.Fatalf("old rate used: %s", converted.Converted)
	}
	ratesResponse := call(http.MethodGet, "/currency/rates", "")
	assertStatus(ratesResponse, 200)
	var rates []interface{}
	if err := json.Unmarshal(ratesResponse.Body.Bytes(), &rates); err != nil {
		t.Fatal(err)
	}
	if len(rates) != 1 {
		t.Fatalf("duplicate currency rates: %d", len(rates))
	}

	// Concurrent distinct users must not lose aggregate increments.
	var userIDs []uint
	for i := 0; i < 10; i++ {
		other := models.User{Username: fmt.Sprintf("concurrent_%d", i), Password: "unused"}
		if err := db.Create(&other).Error; err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, other.ID)
	}
	var wg sync.WaitGroup
	errorsCh := make(chan error, len(userIDs))
	for _, userID := range userIDs {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			_, err := utils.ToggleLikeInDB(a.ID, id, 1)
			errorsCh <- err
		}(userID)
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent like: %v", err)
		}
	}
	if err := db.First(&a, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if a.LikesCount != 11 {
		t.Fatalf("lost concurrent likes: %d", a.LikesCount)
	}

	// An old indexing task must not change the status of a newer article version.
	if err := db.Model(&b).Updates(map[string]interface{}{
		"rag_version": 2, "content_hash": "new", "rag_status": utils.RAGStatusPending,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if utils.MarkArticleRAGIndexed(b.ID, 1, "old") {
		t.Fatal("stale index task marked newer article indexed")
	}
	var current models.Article
	if err := db.First(&current, b.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.RagStatus != utils.RAGStatusPending {
		t.Fatalf("stale status: %s", current.RagStatus)
	}

	keyword := models.Article{Title: "currency outlook", Preview: "currency outlook", Content: "currency outlook for traders", UserID: user.ID, Username: user.Username}
	if err := db.Create(&keyword).Error; err != nil {
		t.Fatal(err)
	}
	hits, err := utils.RetrieveRAGHits("currency", 5)
	if err != nil || len(hits) == 0 || hits[0].ArticleID != keyword.ID || hits[0].Source != "keyword" {
		t.Fatalf("MySQL RAG fallback failed: hits=%+v err=%v", hits, err)
	}
}
