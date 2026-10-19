package utils

import (
	"back/config"
	"back/global"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
	blacklistPrefix  = "jwt:blacklist:"
	refreshPrefix    = "jwt:refresh:"
)

var ErrTokenStoreUnavailable = errors.New("token store unavailable")

// TokenClaims 解析后的 JWT 声明
type TokenClaims struct {
	Username string
	UserID   uint
	JTI      string
	Type     string
}

// TokenPair 登录/刷新返回的双令牌
type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func newJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func jwtSecret() []byte {
	return []byte(config.GetJWTSecret())
}

func accessTTL() time.Duration {
	m := config.GetJWTAccessExpireMinutes()
	if m <= 0 {
		m = 120
	}
	return time.Duration(m) * time.Minute
}

func refreshTTL() time.Duration {
	d := config.GetJWTRefreshExpireDays()
	if d <= 0 {
		d = 7
	}
	return time.Duration(d) * 24 * time.Hour
}

func signToken(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

func parseTokenRaw(tokenString string) (jwt.MapClaims, error) {
	if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
		tokenString = tokenString[7:]
	}
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func claimsToTokenClaims(claims jwt.MapClaims, expectedType string) (*TokenClaims, error) {
	typ, _ := claims["typ"].(string)
	if expectedType != "" && typ != expectedType {
		return nil, fmt.Errorf("invalid token type: %s", typ)
	}
	username, ok := claims["username"].(string)
	if !ok {
		return nil, errors.New("username claim missing")
	}
	userIDVal, ok := claims["user_id"].(float64)
	if !ok {
		return nil, errors.New("user_id claim missing")
	}
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return nil, errors.New("jti claim missing")
	}
	return &TokenClaims{
		Username: username,
		UserID:   uint(userIDVal),
		JTI:      jti,
		Type:     typ,
	}, nil
}

// GenerateTokenPair 签发 access + refresh 双令牌
func GenerateTokenPair(username string, userID uint) (*TokenPair, error) {
	pair, refreshJTI, err := newSignedTokenPair(username, userID)
	if err != nil {
		return nil, err
	}
	if err := storeRefreshToken(refreshJTI, username, userID, refreshTTL()); err != nil {
		return nil, err
	}
	return pair, nil
}

// newSignedTokenPair does not publish the refresh token. Rotation publishes it
// in the same Redis operation that consumes the previous token.
func newSignedTokenPair(username string, userID uint) (*TokenPair, string, error) {
	accessJTI, err := newJTI()
	if err != nil {
		return nil, "", err
	}
	refreshJTI, err := newJTI()
	if err != nil {
		return nil, "", err
	}

	now := time.Now()
	accessExp := now.Add(accessTTL())
	refreshExp := now.Add(refreshTTL())

	accessClaims := jwt.MapClaims{
		"username": username,
		"user_id":  userID,
		"jti":      accessJTI,
		"typ":      tokenTypeAccess,
		"iat":      now.Unix(),
		"exp":      accessExp.Unix(),
	}
	accessSigned, err := signToken(accessClaims)
	if err != nil {
		return nil, "", err
	}

	refreshClaims := jwt.MapClaims{
		"username": username,
		"user_id":  userID,
		"jti":      refreshJTI,
		"typ":      tokenTypeRefresh,
		"iat":      now.Unix(),
		"exp":      refreshExp.Unix(),
	}
	refreshSigned, err := signToken(refreshClaims)
	if err != nil {
		return nil, "", err
	}

	return &TokenPair{
		AccessToken:  "Bearer " + accessSigned,
		RefreshToken: refreshSigned,
	}, refreshJTI, nil
}

func storeRefreshToken(jti, username string, userID uint, ttl time.Duration) error {
	if global.RedisDB == nil {
		return ErrTokenStoreUnavailable
	}
	val := fmt.Sprintf("%s:%d", username, userID)
	if err := global.RedisDB.Set(refreshPrefix+jti, val, ttl).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrTokenStoreUnavailable, err)
	}
	return nil
}

func refreshTokenExists(jti string) (bool, error) {
	if global.RedisDB == nil {
		return false, ErrTokenStoreUnavailable
	}
	n, err := global.RedisDB.Exists(refreshPrefix + jti).Result()
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrTokenStoreUnavailable, err)
	}
	return n > 0, nil
}

func deleteRefreshToken(jti string) error {
	if global.RedisDB == nil {
		return ErrTokenStoreUnavailable
	}
	return global.RedisDB.Del(refreshPrefix + jti).Err()
}

func tokenBlacklisted(jti string) (bool, error) {
	if global.RedisDB == nil {
		return false, ErrTokenStoreUnavailable
	}
	n, err := global.RedisDB.Exists(blacklistPrefix + jti).Result()
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrTokenStoreUnavailable, err)
	}
	return n > 0, nil
}

// ParseAccessToken 解析并校验 access token（含黑名单）
func ParseAccessToken(tokenString string) (*TokenClaims, error) {
	claims, err := parseTokenRaw(tokenString)
	if err != nil {
		return nil, err
	}
	tc, err := claimsToTokenClaims(claims, tokenTypeAccess)
	if err != nil {
		return nil, err
	}
	blacklisted, err := tokenBlacklisted(tc.JTI)
	if err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, errors.New("token revoked")
	}
	return tc, nil
}

// ParseRefreshToken 解析 refresh token
func ParseRefreshToken(tokenString string) (*TokenClaims, error) {
	claims, err := parseTokenRaw(tokenString)
	if err != nil {
		return nil, err
	}
	tc, err := claimsToTokenClaims(claims, tokenTypeRefresh)
	if err != nil {
		return nil, err
	}
	blacklisted, err := tokenBlacklisted(tc.JTI)
	if err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, errors.New("token revoked")
	}
	exists, err := refreshTokenExists(tc.JTI)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("refresh token not found or expired")
	}
	return tc, nil
}

// BlacklistToken 将 jti 加入黑名单，ttl 为 token 剩余有效期
func BlacklistToken(jti string, ttl time.Duration) error {
	if jti == "" {
		return nil
	}
	if global.RedisDB == nil {
		return ErrTokenStoreUnavailable
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return global.RedisDB.Set(blacklistPrefix+jti, "1", ttl).Err()
}

// IsTokenBlacklisted 检查 jti 是否在黑名单
func IsTokenBlacklisted(jti string) bool {
	if jti == "" {
		return false
	}
	blacklisted, err := tokenBlacklisted(jti)
	return err == nil && blacklisted
}

// RevokeRefreshToken 吊销 refresh token
func RevokeRefreshToken(jti string) error {
	return deleteRefreshToken(jti)
}

// BlacklistTokenByClaims 根据 claims 中的 exp 计算 TTL 并拉黑
func BlacklistTokenByClaims(claims jwt.MapClaims) error {
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return nil
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return BlacklistToken(jti, accessTTL())
	}
	ttl := time.Until(time.Unix(int64(exp), 0))
	return BlacklistToken(jti, ttl)
}

// RefreshTokens 用 refresh token 换取新的 token 对（轮换 refresh）
func RefreshTokens(refreshToken string) (*TokenPair, error) {
	claims, err := parseTokenRaw(refreshToken)
	if err != nil {
		return nil, err
	}
	tc, err := claimsToTokenClaims(claims, tokenTypeRefresh)
	if err != nil {
		return nil, err
	}
	if global.RedisDB == nil {
		return nil, ErrTokenStoreUnavailable
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, errors.New("refresh token expiration missing")
	}
	ttl := time.Until(time.Unix(int64(exp), 0))
	if ttl <= 0 {
		return nil, errors.New("refresh token expired")
	}
	nextPair, nextJTI, err := newSignedTokenPair(tc.Username, tc.UserID)
	if err != nil {
		return nil, err
	}
	// Consume the old token, blacklist it, and persist the newly signed token
	// together. Exactly one concurrent request can win the rotation; a failed
	// Redis operation cannot strand the user between consumption and issuance.
	result, err := global.RedisDB.Eval(`
local value = redis.call('GET', KEYS[1])
if not value or value ~= ARGV[1] or redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
if redis.call('EXISTS', KEYS[3]) == 1 then return -1 end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], '1', 'PX', ARGV[2])
redis.call('SET', KEYS[3], ARGV[1], 'PX', ARGV[3])
return 1
`, []string{refreshPrefix + tc.JTI, blacklistPrefix + tc.JTI, refreshPrefix + nextJTI},
		fmt.Sprintf("%s:%d", tc.Username, tc.UserID), ttl.Milliseconds(), refreshTTL().Milliseconds()).Int()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenStoreUnavailable, err)
	}
	if result != 1 {
		return nil, errors.New("refresh token already used or revoked")
	}
	return nextPair, nil
}

// LogoutTokens 登出：拉黑 access jti，吊销 refresh
func LogoutTokens(accessJTI, refreshToken string) error {
	var refreshJTI string
	if refreshToken != "" {
		if claims, err := parseTokenRaw(refreshToken); err == nil {
			if tc, err := claimsToTokenClaims(claims, tokenTypeRefresh); err == nil {
				refreshJTI = tc.JTI
			}
		}
	}
	if accessJTI == "" && refreshJTI == "" {
		return nil
	}
	if global.RedisDB == nil {
		return ErrTokenStoreUnavailable
	}
	// Apply both revocations together; a failure must not leave an old refresh
	// token usable after its access token has already been blacklisted.
	if err := global.RedisDB.Eval(`
if ARGV[1] ~= '' then redis.call('SET', KEYS[1], '1', 'PX', ARGV[3]) end
if ARGV[2] ~= '' then
  redis.call('DEL', KEYS[2])
  redis.call('SET', KEYS[3], '1', 'PX', ARGV[4])
end
return 1
`, []string{blacklistPrefix + accessJTI, refreshPrefix + refreshJTI, blacklistPrefix + refreshJTI},
		accessJTI, refreshJTI, accessTTL().Milliseconds(), refreshTTL().Milliseconds()).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrTokenStoreUnavailable, err)
	}
	return nil
}

// ParseJWT 兼容旧调用：仅解析 access token
func ParseJWT(tokenString string) (string, uint, error) {
	tc, err := ParseAccessToken(tokenString)
	if err != nil {
		return "", 0, err
	}
	return tc.Username, tc.UserID, nil
}

// GenerateJWT 兼容旧调用：改为返回 access token 字符串
func GenerateJWT(username string, userID uint) (string, error) {
	pair, err := GenerateTokenPair(username, userID)
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}
