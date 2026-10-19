package controllers_test

import (
	"back/config"
	"back/controllers"
	"back/global"
	"back/middlewares"
	"back/models"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Separate Redis DB from the utils package's integration tests, which Go may
// run in parallel. This test owns only its uniquely named MySQL schema and
// exact Redis keys created by its requests.
func TestAuthHTTPIntegration(t *testing.T) {
	dsn, addr := os.Getenv("TEST_MYSQL_ADMIN_DSN"), os.Getenv("TEST_REDIS_ADDR")
	redisDB, err := strconv.Atoi(os.Getenv("TEST_REDIS_AUTH_DB"))
	if dsn == "" || addr == "" || err != nil || redisDB < 0 {
		t.Skip("set TEST_MYSQL_ADMIN_DSN, TEST_REDIS_ADDR and isolated TEST_REDIS_AUTH_DB")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: redisDB})
	if err := client.Ping().Err(); err != nil {
		t.Fatal(err)
	}
	size, err := client.DBSize().Result()
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		_ = client.Close()
		t.Skipf("isolated auth Redis DB is not empty (%d keys)", size)
	}
	var redisKeys []string
	t.Cleanup(func() {
		if len(redisKeys) > 0 {
			_ = client.Del(redisKeys...).Err()
		}
		_ = client.Close()
	})

	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("forum_auth_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(schema, "forum_auth_test_") {
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
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	previousDB, previousRedis := global.Db, global.RedisDB
	global.Db, global.RedisDB = db, client
	t.Cleanup(func() { global.Db, global.RedisDB = previousDB, previousRedis })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/register", controllers.Register)
	router.GET("/captcha", controllers.GetCaptcha)
	router.POST("/login", controllers.Login)
	router.POST("/refresh", controllers.Refresh)
	auth := router.Group("/")
	auth.Use(middlewares.AuthMiddleware())
	auth.POST("/logout", controllers.Logout)
	auth.GET("/me", func(c *gin.Context) { c.Status(http.StatusOK) })
	call := func(method, path, body, access string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if access != "" {
			request.Header.Set("Authorization", access)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	assertStatus := func(response *httptest.ResponseRecorder, want int) {
		t.Helper()
		if response.Code != want {
			t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
		}
	}
	type tokenPair struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	decodePair := func(response *httptest.ResponseRecorder) tokenPair {
		t.Helper()
		var pair tokenPair
		if err := json.Unmarshal(response.Body.Bytes(), &pair); err != nil {
			t.Fatal(err)
		}
		if pair.AccessToken == "" || pair.RefreshToken == "" {
			t.Fatal("missing tokens")
		}
		for _, raw := range []string{pair.AccessToken, pair.RefreshToken} {
			raw = strings.TrimPrefix(raw, "Bearer ")
			token, err := jwt.Parse(raw, func(*jwt.Token) (interface{}, error) { return []byte(config.GetJWTSecret()), nil })
			if err != nil {
				t.Fatal(err)
			}
			claims := token.Claims.(jwt.MapClaims)
			jti, _ := claims["jti"].(string)
			redisKeys = append(redisKeys, "jwt:refresh:"+jti, "jwt:blacklist:"+jti)
		}
		return pair
	}

	registered := call(http.MethodPost, "/register", `{"username":"auth_http_user","password":"secret123"}`, "")
	assertStatus(registered, http.StatusCreated)
	registrationPair := decodePair(registered)
	assertStatus(call(http.MethodGet, "/me", "", registrationPair.AccessToken), http.StatusOK)
	var user models.User
	if err := db.Where("username = ?", "auth_http_user").First(&user).Error; err != nil || user.Password == "secret123" {
		t.Fatalf("registration did not persist a hashed password: %v", err)
	}

	captchaResponse := call(http.MethodGet, "/captcha", "", "")
	assertStatus(captchaResponse, http.StatusOK)
	var challenge struct {
		ID    string `json:"captchaId"`
		Image string `json:"captchaImage"`
	}
	if err := json.Unmarshal(captchaResponse.Body.Bytes(), &challenge); err != nil || challenge.ID == "" || challenge.Image == "" {
		t.Fatalf("invalid captcha response: %v", err)
	}
	redisKeys = append(redisKeys, "captcha:"+challenge.ID)
	answer, err := client.Get("captcha:" + challenge.ID).Result()
	if err != nil {
		t.Fatal(err)
	}
	loginBody := fmt.Sprintf(`{"username":"auth_http_user","password":"secret123","captchaId":%q,"captchaValue":%q}`, challenge.ID, answer)
	loggedIn := call(http.MethodPost, "/login", loginBody, "")
	assertStatus(loggedIn, http.StatusOK)
	loginPair := decodePair(loggedIn)
	assertStatus(call(http.MethodPost, "/login", loginBody, ""), http.StatusBadRequest)

	refreshBody := fmt.Sprintf(`{"refreshToken":%q}`, loginPair.RefreshToken)
	refreshed := call(http.MethodPost, "/refresh", refreshBody, "")
	assertStatus(refreshed, http.StatusOK)
	newPair := decodePair(refreshed)
	assertStatus(call(http.MethodPost, "/refresh", refreshBody, ""), http.StatusUnauthorized)
	assertStatus(call(http.MethodGet, "/me", "", newPair.AccessToken), http.StatusOK)
	logoutBody := fmt.Sprintf(`{"refreshToken":%q}`, newPair.RefreshToken)
	assertStatus(call(http.MethodPost, "/logout", logoutBody, newPair.AccessToken), http.StatusOK)
	assertStatus(call(http.MethodGet, "/me", "", newPair.AccessToken), http.StatusUnauthorized)
	assertStatus(call(http.MethodPost, "/refresh", logoutBody, ""), http.StatusUnauthorized)
}
