package utils

import (
	"back/global"
	"os"
	"strconv"
	"testing"

	"github.com/go-redis/redis"
)

func TestBloomMissingBitmapFallsBack(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR and TEST_REDIS_DB for isolated Redis integration test")
	}
	db, err := strconv.Atoi(os.Getenv("TEST_REDIS_DB"))
	if err != nil || db < 0 {
		t.Fatal("TEST_REDIS_DB must be an isolated numeric database")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	defer client.Close()
	if err := client.Ping().Err(); err != nil {
		t.Fatal(err)
	}
	size, err := client.DBSize().Result()
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Skipf("isolated Redis DB is not empty (%d keys)", size)
	}
	previous := global.RedisDB
	global.RedisDB = client
	defer func() { global.RedisDB = previous }()
	defer client.Del(articleBloomReadyKey, articleBloomKey, articleBloomBuildKey)
	if !MightContainArticle("123") {
		t.Fatal("missing bitmap caused a false negative")
	}
	if err := client.Set(articleBloomReadyKey, "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.SetBit(articleBloomKey, articleBloomSize, 1).Err(); err != nil {
		t.Fatal(err)
	}
	if MightContainArticle("123") {
		t.Fatal("ready filter did not reject absent ID")
	}
	if err := client.Del(articleBloomKey).Err(); err != nil {
		t.Fatal(err)
	}
	if !MightContainArticle("123") {
		t.Fatal("evicted bitmap caused a false negative")
	}
	AddArticleToBloom("999")
	if !MightContainArticle("123") {
		t.Fatal("partial bitmap was used after eviction")
	}
}
