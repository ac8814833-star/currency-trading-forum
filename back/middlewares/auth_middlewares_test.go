package middlewares

import (
	"back/config"
	"back/global"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestAuthMiddlewareReportsTokenStoreOutage(t *testing.T) {
	previous := global.RedisDB
	global.RedisDB = nil
	t.Cleanup(func() { global.RedisDB = previous })
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "test", "user_id": 1, "jti": "middleware-test", "typ": "access",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(config.GetJWTSecret()))
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503: %s", response.Code, response.Body.String())
	}
}
