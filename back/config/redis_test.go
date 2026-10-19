package config

import (
	"back/global"
	"testing"
)

func TestInitRedisKeepsConfiguredClientOnInitialOutage(t *testing.T) {
	previousConfig, previousClient := AppConfig, global.RedisDB
	AppConfig = &config{}
	AppConfig.Redis.Addr = "127.0.0.1:1"
	t.Cleanup(func() {
		if global.RedisDB != nil {
			_ = global.RedisDB.Close()
		}
		AppConfig, global.RedisDB = previousConfig, previousClient
	})
	InitRedis()
	if global.RedisDB == nil {
		t.Fatal("configured Redis outage was mistaken for DB-only mode")
	}
	if err := global.RedisDB.Ping().Err(); err == nil {
		t.Fatal("test address unexpectedly reachable")
	}
}
