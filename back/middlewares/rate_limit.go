package middlewares

import (
	"back/global"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Token bucket 限流中间件（基于 Redis）
// 可按 IP 或按用户（如果已登录）进行限流

type RateLimitConfig struct {
	KeyPrefix      string        // 限流 key 前缀，例如 "rate:login"、"rate:comment"
	BucketCapacity int64         // 令牌桶容量（最大令牌数）
	RefillRate     float64       // 每秒填充多少令牌（令牌生成速率）
	BlockedMsg     string        // 被限流时的提示信息
	BlockedStatus  int           // 被限流时的 HTTP 状态码
	Expire         time.Duration // Redis key 过期时间
}

// RateLimitMiddleware 返回一个可配置的限流中间件
func RateLimitMiddleware(cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if global.RedisDB == nil {
			// 没有 Redis 时，不做限流
			c.Next()
			return
		}

		identifier := getClientIdentifier(c)
		key := cfg.KeyPrefix + ":" + identifier

		now := time.Now().Unix()

		// 调试日志：查看限流 Key
		// fmt.Printf("[RateLimit] Key: %s, Capacity: %d, Rate: %f\n", key, cfg.BucketCapacity, cfg.RefillRate)

		script := `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local data = redis.call("HMGET", key, "tokens", "timestamp")
local tokens = tonumber(data[1])
local last_ts = tonumber(data[2])

if tokens == nil then
  -- 第一次访问，初始化桶
  -- 注意：这里设为 capacity 而不是 capacity - 1，确保第一次请求通过且剩下 capacity-1 个
  tokens = capacity - 1
  last_ts = now
  redis.call("HMSET", key, "tokens", tokens, "timestamp", last_ts)
  redis.call("EXPIRE", key, ttl)
  return {1, tokens}
else
  -- 计算这段时间内产生的令牌
  local delta = math.max(0, now - last_ts)
  local refill = delta * refill_rate
  tokens = math.min(capacity, tokens + refill)
  
  if tokens >= 1 then
    tokens = tokens - 1
    -- 只有扣减成功才更新时间戳，防止频繁请求导致时间戳一直刷新而无法生成新令牌
    redis.call("HMSET", key, "tokens", tokens, "timestamp", now)
    redis.call("EXPIRE", key, ttl)
    return {1, tokens}
  else
    return {0, tokens}
  end
end
`

		// 过期时间，默认 1 小时
		ttl := int64(cfg.Expire.Seconds())
		if ttl <= 0 {
			ttl = 3600
		}

		result, err := global.RedisDB.Eval(script, []string{key},
			cfg.BucketCapacity,
			cfg.RefillRate,
			now,
			ttl,
		).Result()
		if err != nil {
			// Redis 出错时，放行请求
			c.Next()
			return
		}

		arr, ok := result.([]interface{})
		if !ok || len(arr) < 1 {
			c.Next()
			return
		}

		var allowed int64
		switch v := arr[0].(type) {
		case int64:
			allowed = v
		case float64:
			allowed = int64(v)
		case string:
			allowed, _ = strconv.ParseInt(v, 10, 64)
		case []byte:
			allowed, _ = strconv.ParseInt(string(v), 10, 64)
		default:
			allowed = 0
		}
		if allowed == 0 {
			status := cfg.BlockedStatus
			if status == 0 {
				status = http.StatusTooManyRequests
			}
			msg := cfg.BlockedMsg
			if msg == "" {
				msg = "Too many requests, please try again later"
			}
			c.JSON(status, gin.H{"error": msg})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getClientIdentifier 获取客户端标识，可根据需要调整（IP/用户）
func getClientIdentifier(c *gin.Context) string {
	// 如果有登录用户，则优先按用户名限流
	if username, exists := c.Get("username"); exists {
		if s, ok := username.(string); ok && s != "" {
			return "user:" + s
		}
	}

	// 否则按 IP 限流
	ip := c.ClientIP()
	// 处理 IPv6/IPv4 混合格式
	if strings.Contains(ip, ":") {
		host, _, err := net.SplitHostPort(ip)
		if err == nil {
			ip = host
		}
	}
	return "ip:" + ip
}

// toString 辅助函数，将 Lua 返回的 interface{} 转为 string
func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}
