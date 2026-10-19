package utils

import (
	"back/global"
	"back/models"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis"
	mysqldriver "github.com/go-sql-driver/mysql"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This test verifies the hot path without requiring RabbitMQ: Redis accepts
// state + outbox atomically, and the same events remain correct when MySQL
// consumers observe them in reverse order.
func TestAsyncLikeRedisOutboxAndVersioning(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_ADMIN_DSN")
	addr := os.Getenv("TEST_REDIS_ADDR")
	dbNumber, redisErr := strconv.Atoi(os.Getenv("TEST_REDIS_DB"))
	if dsn == "" || addr == "" || redisErr != nil || dbNumber < 0 {
		t.Skip("set TEST_MYSQL_ADMIN_DSN, TEST_REDIS_ADDR and isolated TEST_REDIS_DB")
	}

	client := redis.NewClient(&redis.Options{Addr: addr, DB: dbNumber})
	t.Cleanup(func() {
		deleteLikeTestKeys(t, client)
		_ = client.Close()
	})
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

	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("forum_like_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(schema, "forum_like_test_") {
		t.Fatal("unsafe schema name")
	}
	if err := admin.Exec("CREATE DATABASE " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE " + schema).Error; err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
	})
	parsed.DBName = schema
	parsed.ParseTime = true
	db, err := gorm.Open(mysql.Open(parsed.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Article{}, &models.Like{}); err != nil {
		t.Fatal(err)
	}
	previousDB, previousRedis := global.Db, global.RedisDB
	global.Db, global.RedisDB = db, client
	t.Cleanup(func() {
		global.Db, global.RedisDB = previousDB, previousRedis
	})

	user := models.User{Username: "async_like_user", Password: "unused"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	article := models.Article{Title: "async", Preview: "async preview", Content: "async content", UserID: user.ID, Username: user.Username}
	if err := db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}

	InitLikeCacheInfrastructure()
	if count, err := ToggleLikeAsync(article.ID, user.ID, 1); err != nil || count != 1 {
		t.Fatalf("like count=%d err=%v", count, err)
	}
	if _, err := ToggleLikeAsync(article.ID, user.ID, 1); !errors.Is(err, ErrAlreadyLiked) {
		t.Fatalf("duplicate like err=%v", err)
	}
	if count, err := ToggleLikeAsync(article.ID, user.ID, -1); err != nil || count != 0 {
		t.Fatalf("unlike count=%d err=%v", count, err)
	}
	if count, err := ToggleLikeAsync(article.ID, user.ID, 1); err != nil || count != 1 {
		t.Fatalf("re-like count=%d err=%v", count, err)
	}

	// Distinct users exercise the hot path concurrently. No article-row lock is
	// taken here: each request only executes the Redis Lua script.
	const concurrentUsers = 24
	userIDs := make([]uint, 0, concurrentUsers)
	for i := 0; i < concurrentUsers; i++ {
		other := models.User{Username: fmt.Sprintf("async_like_user_%d", i), Password: "unused"}
		if err := db.Create(&other).Error; err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, other.ID)
	}
	var wg sync.WaitGroup
	errorsCh := make(chan error, len(userIDs))
	for _, userID := range userIDs {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			_, err := ToggleLikeAsync(article.ID, id, 1)
			errorsCh <- err
		}(userID)
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent async like: %v", err)
		}
	}

	count, liked, err := GetLikeState(article.ID, user.ID)
	if err != nil || count != concurrentUsers+1 || !liked {
		t.Fatalf("redis state count=%d liked=%t err=%v", count, liked, err)
	}
	ranks, err := GetHotLikeRanks(10)
	if err != nil || len(ranks) != 1 || ranks[0].ArticleID != article.ID || ranks[0].Likes != concurrentUsers+1 {
		t.Fatalf("ranking=%+v err=%v", ranks, err)
	}
	if length, err := client.LLen(likeOutboxList).Result(); err != nil || length != concurrentUsers+3 {
		t.Fatalf("outbox len=%d err=%v", length, err)
	}

	FlushLikeCounters()
	if err := db.First(&article, article.ID).Error; err != nil {
		t.Fatal(err)
	}
	if article.LikesCount != concurrentUsers+1 {
		t.Fatalf("batched article count=%d", article.LikesCount)
	}

	// A partial bitmap eviction must not rebuild from MySQL while the accepted
	// events are still only in the outbox/MQ path.
	if err := client.Del(likeUsersKey(article.ID)).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := ToggleLikeAsync(article.ID, user.ID, 1); !errors.Is(err, ErrLikeCacheWarming) {
		t.Fatalf("lost bitmap accepted a request before persistence: %v", err)
	}
	if _, _, err := GetLikeState(article.ID, user.ID); !errors.Is(err, ErrLikeCacheWarming) {
		t.Fatalf("lost bitmap exposed a stale database read: %v", err)
	}
	if err := client.Del(likeRankingKey).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := GetHotLikeRanks(10); !errors.Is(err, ErrLikeCacheWarming) {
		t.Fatalf("lost ranking exposed a stale database rank: %v", err)
	}

	entries, err := client.LRange(likeOutboxList, 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		msg, err := likeMessageFromOutbox(entry)
		if err != nil {
			t.Fatal(err)
		}
		if err := ApplyLikeMessage(msg); err != nil {
			t.Fatal(err)
		}
	}
	var persisted models.Like
	if err := db.Unscoped().Where("article_id = ? AND user_id = ?", article.ID, user.ID).First(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.DeletedAt.Valid || persisted.StateVersion <= 0 {
		t.Fatalf("out-of-order persistence lost final state: %+v", persisted)
	}
	if pending, err := client.SCard(likePendingEventsKey(article.ID)).Result(); err != nil || pending != 0 {
		t.Fatalf("pending event set=%d err=%v", pending, err)
	}
	if err := client.Del(likeEventSequenceKey).Err(); err != nil {
		t.Fatal(err)
	}
	if err := ensureLikeEventSequence(); err != nil {
		t.Fatal(err)
	}
	if version, err := client.Get(likeEventSequenceKey).Int64(); err != nil || version < concurrentUsers+3 {
		t.Fatalf("event sequence was reset after partial eviction: version=%d err=%v", version, err)
	}
	if _, err := ToggleLikeAsync(article.ID, user.ID, 1); !errors.Is(err, ErrAlreadyLiked) {
		t.Fatalf("cache rebuild lost persisted like: %v", err)
	}
	if err := client.Del(likeRankingKey).Err(); err != nil {
		t.Fatal(err)
	}
	if ranks, err := GetHotLikeRanks(10); err != nil || len(ranks) != 1 || ranks[0].Likes != concurrentUsers+1 {
		t.Fatalf("ranking rebuild after partial eviction: ranks=%+v err=%v", ranks, err)
	}
}

// This optional test exercises the full Redis outbox -> RabbitMQ confirm ->
// multi-consumer MySQL persistence path against local disposable resources.
func TestAsyncLikeRabbitMQEndToEnd(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_ADMIN_DSN")
	addr := os.Getenv("TEST_REDIS_ADDR")
	rabbitURL := os.Getenv("TEST_RABBITMQ_URL")
	dbNumber, redisErr := strconv.Atoi(os.Getenv("TEST_REDIS_DB"))
	if dsn == "" || addr == "" || rabbitURL == "" || redisErr != nil || dbNumber < 0 {
		t.Skip("set TEST_MYSQL_ADMIN_DSN, TEST_REDIS_ADDR, TEST_REDIS_DB and TEST_RABBITMQ_URL")
	}

	client := redis.NewClient(&redis.Options{Addr: addr, DB: dbNumber})
	t.Cleanup(func() {
		deleteLikeTestKeys(t, client)
		_ = client.Close()
	})
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

	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("forum_like_mq_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(schema, "forum_like_mq_test_") {
		t.Fatal("unsafe schema name")
	}
	if err := admin.Exec("CREATE DATABASE " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE " + schema).Error; err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
	})
	parsed.DBName = schema
	parsed.ParseTime = true
	db, err := gorm.Open(mysql.Open(parsed.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Article{}, &models.Like{}); err != nil {
		t.Fatal(err)
	}

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatal(err)
	}
	queueName := fmt.Sprintf("likes_sync_test_%d", time.Now().UnixNano())
	t.Setenv("LIKE_QUEUE_NAME", queueName)
	previousDB, previousRedis, previousConn := global.Db, global.RedisDB, global.RabbitMQConn
	global.Db, global.RedisDB, global.RabbitMQConn = db, client, conn
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
		cleanup, err := conn.Channel()
		if err == nil {
			_, _ = cleanup.QueueDelete(queueName, false, false, false)
			_ = cleanup.Close()
		}
		_ = conn.Close()
		global.Db, global.RedisDB, global.RabbitMQConn = previousDB, previousRedis, previousConn
	})

	user := models.User{Username: "async_like_mq_user", Password: "unused"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	article := models.Article{Title: "mq", Preview: "mq preview", Content: "mq content", UserID: user.ID, Username: user.Username}
	if err := db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}

	InitLikeCacheInfrastructure()
	StartLikeOutboxRelay(ctx)
	if count, err := ToggleLikeAsync(article.ID, user.ID, 1); err != nil || count != 1 {
		t.Fatalf("like count=%d err=%v", count, err)
	}
	// Start the relay before consumers. The relay itself must declare the queue;
	// otherwise a default-exchange publish could be confirmed and silently
	// discarded when startup scheduling places the relay first.
	deadline := time.Now().Add(5 * time.Second)
	var inspectErr error
	for {
		// A passive QueueInspect against a not-yet-declared queue closes its AMQP
		// channel by protocol. Use a fresh inspection channel for each retry so
		// the expected initial 404 cannot poison later checks.
		inspect, channelErr := conn.Channel()
		if channelErr != nil {
			t.Fatal(channelErr)
		}
		state, currentInspectErr := inspect.QueueInspect(queueName)
		_ = inspect.Close()
		inspectErr = currentInspectErr
		if inspectErr == nil && state.Messages == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("relay did not declare and fill queue, last error=%v", inspectErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	StartLikeWorker(ctx)

	deadline = time.Now().Add(8 * time.Second)
	for {
		var persisted models.Like
		result := db.Unscoped().Where("article_id = ? AND user_id = ?", article.ID, user.ID).Limit(1).Find(&persisted)
		if result.Error == nil && result.RowsAffected == 1 && !persisted.DeletedAt.Valid && persisted.StateVersion > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("RabbitMQ event was not persisted, last error=%v", result.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pending, err := client.LLen(likeOutboxList).Result(); err != nil || pending != 0 {
		t.Fatalf("outbox pending=%d err=%v", pending, err)
	}
	if processing, err := client.LLen(likeOutboxProcessingList).Result(); err != nil || processing != 0 {
		t.Fatalf("outbox processing=%d err=%v", processing, err)
	}
}

func deleteLikeTestKeys(t *testing.T, client *redis.Client) {
	t.Helper()
	var cursor uint64
	for {
		keys, next, err := client.Scan(cursor, likeKeyPrefix+"*", 100).Result()
		if err != nil {
			t.Errorf("scan like test keys: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := client.Del(keys...).Err(); err != nil {
				t.Errorf("delete like test keys: %v", err)
				return
			}
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}
