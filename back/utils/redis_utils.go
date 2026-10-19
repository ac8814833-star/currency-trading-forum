package utils

import (
	"back/global"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-redis/redis"
)

// SetCache 设置缓存，带过期时间
func SetCache(key string, value interface{}, expiration time.Duration) error {
	if global.RedisDB == nil {
		return redis.Nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return global.RedisDB.Set(key, data, expiration).Err()
}

// GetCache 获取缓存
func GetCache(key string, dest interface{}) error {
	if global.RedisDB == nil {
		return redis.Nil
	}
	data, err := global.RedisDB.Get(key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

// DeleteCache 删除缓存
func DeleteCache(key string) error {
	if global.RedisDB == nil {
		return nil
	}
	return global.RedisDB.Del(key).Err()
}

// IncrementCounter 增加计数器
func IncrementCounter(key string) (int64, error) {
	if global.RedisDB == nil {
		return 0, redis.Nil
	}
	return global.RedisDB.Incr(key).Result()
}

// GetCounter 获取计数器值
func GetCounter(key string) (int64, error) {
	if global.RedisDB == nil {
		return 0, redis.Nil
	}
	val, err := global.RedisDB.Get(key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count, err := strconv.ParseInt(val, 10, 64)
	return count, err
}

// SetWithExpiration 设置键值对，带过期时间（字符串类型）
func SetWithExpiration(key string, value string, expiration time.Duration) error {
	if global.RedisDB == nil {
		return redis.Nil
	}
	return global.RedisDB.Set(key, value, expiration).Err()
}

// GetString 获取字符串值
func GetString(key string) (string, error) {
	if global.RedisDB == nil {
		return "", redis.Nil
	}
	return global.RedisDB.Get(key).Result()
}

// Exists 检查键是否存在
func Exists(key string) (bool, error) {
	if global.RedisDB == nil {
		return false, redis.Nil
	}
	count, err := global.RedisDB.Exists(key).Result()
	return count > 0, err
}

// SetHash 设置哈希字段
func SetHash(key string, field string, value interface{}) error {
	if global.RedisDB == nil {
		return redis.Nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return global.RedisDB.HSet(key, field, data).Err()
}

// GetHash 获取哈希字段
func GetHash(key string, field string, dest interface{}) error {
	if global.RedisDB == nil {
		return redis.Nil
	}
	data, err := global.RedisDB.HGet(key, field).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

// DeleteHash 删除哈希字段
func DeleteHash(key string, field string) error {
	if global.RedisDB == nil {
		return nil
	}
	return global.RedisDB.HDel(key, field).Err()
}

// AddToSet 添加到集合
func AddToSet(key string, members ...interface{}) error {
	if global.RedisDB == nil {
		return redis.Nil
	}
	return global.RedisDB.SAdd(key, members...).Err()
}

// IsMemberOfSet 检查是否在集合中
func IsMemberOfSet(key string, member interface{}) (bool, error) {
	if global.RedisDB == nil {
		return false, redis.Nil
	}
	return global.RedisDB.SIsMember(key, member).Result()
}

// RemoveFromSet 从集合中移除
func RemoveFromSet(key string, members ...interface{}) error {
	if global.RedisDB == nil {
		return nil
	}
	return global.RedisDB.SRem(key, members...).Err()
}

// SetExpiration 设置键的过期时间
func SetExpiration(key string, expiration time.Duration) error {
	if global.RedisDB == nil {
		return redis.Nil
	}
	return global.RedisDB.Expire(key, expiration).Err()
}

