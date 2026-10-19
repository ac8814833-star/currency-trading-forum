package utils

import (
	"back/global"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis"
	"github.com/mojocn/base64Captcha"
)

var ErrCaptchaUnavailable = errors.New("captcha store unavailable")

// redisStore 实现了 base64Captcha 的 Store 接口
type redisStore struct {
	Client     *redis.Client
	Expiration time.Duration
}

// NewRedisStore 创建一个新的 redisStore 实例，供 base64Captcha 使用
func NewRedisStore() base64Captcha.Store {
	return &redisStore{
		Client:     global.RedisDB,  // 使用全局的 Redis 连接
		Expiration: 5 * time.Minute, // 设置验证码过期时间为5分钟
	}
}

// Set 将验证码的值存储到 Redis 中
func (rs *redisStore) Set(id string, value string) error {
	if rs.Client == nil {
		return ErrCaptchaUnavailable
	}
	return rs.Client.Set("captcha:"+id, value, rs.Expiration).Err()
}

// Get 从 Redis 中检索验证码的值
func (rs *redisStore) Get(id string, clear bool) string {
	if rs.Client == nil {
		return ""
	}
	val, err := rs.Client.Get("captcha:" + id).Result()
	if err != nil {
		return ""
	}
	// 如果 clear 为 true，则在获取后立即删除该验证码
	if clear {
		rs.Client.Del("captcha:" + id)
	}
	return val
}

// Verify 校验验证码答案
func (rs *redisStore) Verify(id, answer string, clear bool) bool {
	val := rs.Get(id, clear)
	return val != "" && val == answer
}

// CreateCaptcha 生成一个新的验证码并将其存储在 Redis 中
func CreateCaptcha() (string, string, error) {
	if global.RedisDB == nil {
		return "", "", ErrCaptchaUnavailable
	}
	store := NewRedisStore() // 使用我们自定义的 Redis 存储
	// 配置验证码驱动（80x240像素，4位数字，0.7的噪点强度，80条干扰线）
	driver := base64Captcha.NewDriverDigit(80, 240, 4, 0.7, 80)
	// 创建验证码实例
	c := base64Captcha.NewCaptcha(driver, store)
	// Generate 方法会自动通过接口调用我们实现的 store.Set() 方法
	id, b64s, _, err := c.Generate()
	return id, b64s, err
}

// VerifyCaptcha 从 Redis 中校验验证码
func VerifyCaptcha(id string, value string) (bool, error) {
	if global.RedisDB == nil {
		return false, ErrCaptchaUnavailable
	}
	if id == "" || value == "" {
		return false, nil
	}
	// Reading and consuming the challenge must be atomic, so concurrent login
	// attempts cannot both reuse a correct answer.
	result, err := global.RedisDB.Eval(`
local answer = redis.call('GET', KEYS[1])
if answer then redis.call('DEL', KEYS[1]) end
return answer or ''
`, []string{"captcha:" + id}).Result()
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrCaptchaUnavailable, err)
	}
	answer, _ := result.(string)
	return answer != "" && answer == value, nil
}
