package config

import (
	"back/global"
	"github.com/go-redis/redis"
	"log"
	"strings"
)

func InitRedis() {
	if AppConfig == nil || strings.TrimSpace(AppConfig.Redis.Addr) == "" {
		global.RedisDB = nil // Only an omitted address enables the DB-only mode.
		log.Println("Redis not configured")
		return
	}
	RedisClient := redis.NewClient(&redis.Options{
		Addr:     AppConfig.Redis.Addr,
		DB:       AppConfig.Redis.DB,
		Password: AppConfig.Redis.Password,
	})

	// Keep the client when the initial ping fails: go-redis reconnects on later
	// commands. A configured outage must not look like a DB-only deployment,
	// especially while accepted like events are still in the Redis outbox.
	global.RedisDB = RedisClient
	_, err := RedisClient.Ping().Result()
	if err != nil {
		log.Printf("Failed to connect to Redis, got error: %v", err)
		log.Printf("Redis-dependent operations will remain unavailable until it reconnects")
		return
	}
	log.Println("Successfully connected to Redis")
}
