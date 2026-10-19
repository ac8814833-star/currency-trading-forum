package utils

import (
	"back/global"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/go-redis/redis"
)

// Run only against a dedicated, initially empty Redis database. No keys
// outside the tokens and captcha created here are removed.
func TestJWTAndCaptchaRedisIntegration(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	dbNumber, err := strconv.Atoi(os.Getenv("TEST_REDIS_DB"))
	if addr == "" || err != nil || dbNumber < 0 {
		t.Skip("set TEST_REDIS_ADDR and isolated TEST_REDIS_DB")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: dbNumber})
	if err := client.Ping().Err(); err != nil {
		t.Fatal(err)
	}
	size, err := client.DBSize().Result()
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		_ = client.Close()
		t.Skipf("isolated Redis DB is not empty (%d keys)", size)
	}
	previous := global.RedisDB
	global.RedisDB = client
	var createdKeys []string
	t.Cleanup(func() {
		if len(createdKeys) > 0 {
			_ = client.Del(createdKeys...).Err()
		}
		global.RedisDB = previous
		_ = client.Close()
	})
	trackPair := func(pair *TokenPair) {
		t.Helper()
		for _, raw := range []string{pair.AccessToken, pair.RefreshToken} {
			claims, err := parseTokenRaw(raw)
			if err != nil {
				t.Fatal(err)
			}
			jti, _ := claims["jti"].(string)
			createdKeys = append(createdKeys, refreshPrefix+jti, blacklistPrefix+jti)
		}
	}

	first, err := GenerateTokenPair("integration-user", 42)
	if err != nil {
		t.Fatal(err)
	}
	trackPair(first)
	if claims, err := ParseAccessToken(first.AccessToken); err != nil || claims.UserID != 42 {
		t.Fatalf("access claims=%+v error=%v", claims, err)
	}
	if _, err := ParseRefreshToken(first.RefreshToken); err != nil {
		t.Fatal(err)
	}

	const concurrent = 16
	type outcome struct {
		pair *TokenPair
		err  error
	}
	outcomes := make(chan outcome, concurrent)
	var wg sync.WaitGroup
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pair, err := RefreshTokens(first.RefreshToken)
			outcomes <- outcome{pair, err}
		}()
	}
	wg.Wait()
	close(outcomes)
	var second *TokenPair
	for result := range outcomes {
		if result.err == nil {
			if second != nil {
				t.Fatal("same refresh token rotated more than once")
			}
			second = result.pair
		}
	}
	if second == nil {
		t.Fatal("no concurrent refresh succeeded")
	}
	trackPair(second)
	if _, err := ParseRefreshToken(first.RefreshToken); err == nil {
		t.Fatal("consumed refresh token remains valid")
	}
	if _, err := ParseRefreshToken(second.RefreshToken); err != nil {
		t.Fatalf("new refresh token invalid: %v", err)
	}
	accessClaims, err := parseTokenRaw(second.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	accessJTI, _ := accessClaims["jti"].(string)
	if err := LogoutTokens(accessJTI, second.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAccessToken(second.AccessToken); err == nil {
		t.Fatal("logged-out access token remains valid")
	}
	if _, err := ParseRefreshToken(second.RefreshToken); err == nil {
		t.Fatal("logged-out refresh token remains valid")
	}

	id, image, err := CreateCaptcha()
	if err != nil || id == "" || image == "" {
		t.Fatalf("captcha id=%q image bytes=%d error=%v", id, len(image), err)
	}
	createdKeys = append(createdKeys, "captcha:"+id)
	answer, err := client.Get("captcha:" + id).Result()
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := VerifyCaptcha(id, answer); err != nil || !valid {
		t.Fatalf("first captcha verification = %t, %v", valid, err)
	}
	if valid, err := VerifyCaptcha(id, answer); err != nil || valid {
		t.Fatalf("replayed captcha verification = %t, %v", valid, err)
	}
}
