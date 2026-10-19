package utils

import (
	"back/global"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ============================================================
// JWT tests without Redis verify that token issuance and revocation fail closed.
// ============================================================

func TestGenerateTokenPair_WithoutRedis(t *testing.T) {
	previous := global.RedisDB
	global.RedisDB = nil
	t.Cleanup(func() { global.RedisDB = previous })
	if _, err := GenerateTokenPair("testuser", 1); !errors.Is(err, ErrTokenStoreUnavailable) {
		t.Fatalf("GenerateTokenPair() error = %v, want unavailable", err)
	}
}

func TestParseAccessToken_WithoutRedis(t *testing.T) {
	previous := global.RedisDB
	global.RedisDB = nil
	t.Cleanup(func() { global.RedisDB = previous })
	token, err := signToken(jwt.MapClaims{
		"username": "alice", "user_id": 42, "jti": "test-access-jti", "typ": tokenTypeAccess,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAccessToken("Bearer " + token); !errors.Is(err, ErrTokenStoreUnavailable) {
		t.Fatalf("ParseAccessToken() error = %v, want unavailable", err)
	}
}

func TestParseAccessToken_InvalidToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"garbage token", "thisIsNotAValidJWTToken"},
		{"no Bearer prefix but looks like JWT", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNFlfCeY1QUEPo2KNJYlI4eGZNsUBP23rBHFfok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAccessToken(tt.token)
			if err == nil {
				t.Error("ParseAccessToken() should return error for invalid token")
			}
		})
	}
}

func TestParseAccessToken_WrongTokenType(t *testing.T) {
	// 用 refresh token 去调 ParseAccessToken → 应该失败（类型不匹配）
	token, err := signToken(jwt.MapClaims{
		"username": "bob", "user_id": 1, "jti": "test-refresh-jti", "typ": tokenTypeRefresh,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseAccessToken(token) // refresh token 当 access 用
	if err == nil {
		t.Fatal("ParseAccessToken() should reject a refresh token")
	}
}

func TestParseAccessToken_ExpiredToken(t *testing.T) {
	// 手动构造一个已过期的 token
	now := time.Now()
	expiredClaims := jwt.MapClaims{
		"username": "expiredUser",
		"user_id":  float64(1),
		"jti":      "test-jti-expired",
		"typ":      "access",
		"iat":      now.Add(-2 * time.Hour).Unix(),
		"exp":      now.Add(-1 * time.Hour).Unix(), // 1 小时前过期
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredToken, err := token.SignedString(jwtSecret())
	if err != nil {
		t.Fatalf("failed to create expired token: %v", err)
	}

	_, err = ParseAccessToken("Bearer " + expiredToken)
	if err == nil {
		t.Fatal("ParseAccessToken() should reject an expired token")
	}
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Logf("Got error (expected expired): %v", err)
	}
}

func TestRefreshTokens_WithoutRedis(t *testing.T) {
	previous := global.RedisDB
	global.RedisDB = nil
	t.Cleanup(func() { global.RedisDB = previous })
	token, err := signToken(jwt.MapClaims{
		"username": "charlie", "user_id": 99, "jti": "test-refresh-jti", "typ": tokenTypeRefresh,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshTokens(token); !errors.Is(err, ErrTokenStoreUnavailable) {
		t.Fatalf("RefreshTokens() error = %v, want unavailable", err)
	}
}

// ============================================================
// 边界条件 — 面试中最容易加分
// ============================================================

func TestBlacklistToken_EmptyJTI(t *testing.T) {
	// 空 JTI 不应该 panic
	err := BlacklistToken("", time.Hour)
	if err != nil {
		t.Errorf("BlacklistToken('') should not error, got: %v", err)
	}
	if IsTokenBlacklisted("") {
		t.Error("IsTokenBlacklisted('') should return false")
	}
}

func TestIsTokenBlacklisted_NonExistent(t *testing.T) {
	// 未加入黑名单的 JTI 应该返回 false
	if IsTokenBlacklisted("this-jti-does-not-exist-12345") {
		t.Error("IsTokenBlacklisted() should return false for unknown JTI")
	}
}
