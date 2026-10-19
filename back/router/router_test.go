package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUntrustedForwardedForIgnored(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	r := SetupRouter()
	r.GET("/_test/client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_test/client-ip", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "127.0.0.1" {
		t.Fatalf("spoofed forwarded IP was trusted: status=%d ip=%q", w.Code, w.Body.String())
	}
}
