package utils

import (
	"back/global"
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
)

// StartLikeStatsSyncWorker batches the redundant articles.likes_count column.
// The request path updates Redis immediately; this worker avoids taking the
// same MySQL article-row lock once for every click.
func StartLikeStatsSyncWorker(ctx context.Context) {
	go func() {
		FlushLikeCounters()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				FlushLikeCounters()
			}
		}
	}()
}

// FlushLikeCounters projects only articles changed since the previous flush.
// A compare-and-delete Lua step leaves an article dirty if another like arrives
// while its older count is being written to MySQL.
func FlushLikeCounters() {
	if global.RedisDB == nil || global.Db == nil {
		return
	}
	pending, err := global.RedisDB.HGetAll(likeDirtyArticlesKey).Result()
	if err != nil {
		logrus.Warnf("read dirty like counters failed: %v", err)
		return
	}
	for articleText, version := range pending {
		articleID, err := strconv.ParseUint(articleText, 10, 64)
		if err != nil || articleID == 0 {
			_ = global.RedisDB.HDel(likeDirtyArticlesKey, articleText).Err()
			continue
		}
		count, err := global.RedisDB.Get(likeCountKey(uint(articleID))).Int64()
		if err == redis.Nil {
			// The cache may be rebuilding after eviction. Keep the dirty marker
			// so a later flush cannot overwrite the database with a false zero.
			continue
		}
		if err != nil {
			logrus.Warnf("read like count for article %d failed: %v", articleID, err)
			continue
		}
		if err := global.Db.Model(&struct{}{}).Table("articles").
			Where("id = ? AND deleted_at IS NULL", uint(articleID)).
			Update("likes_count", count).Error; err != nil {
			logrus.Warnf("flush like count for article %d failed: %v", articleID, err)
			continue
		}
		_ = global.RedisDB.Del("article:detail:" + articleText).Err()
		const clearDirtyScript = `
if redis.call('HGET', KEYS[1], ARGV[1]) == ARGV[2] then
  return redis.call('HDEL', KEYS[1], ARGV[1])
end
return 0
`
		if err := global.RedisDB.Eval(clearDirtyScript, []string{likeDirtyArticlesKey}, articleText, version).Err(); err != nil {
			logrus.Warnf("clear dirty like marker for article %d failed: %v", articleID, err)
		}
	}
}
