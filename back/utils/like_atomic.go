package utils

import (
	"back/config"
	"back/global"
	"back/models"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// LikeRedisConfigured distinguishes an intentional DB-only deployment from a
// configured Redis outage. The latter must not switch sources of truth.
func LikeRedisConfigured() bool {
	return config.AppConfig != nil && strings.TrimSpace(config.AppConfig.Redis.Addr) != ""
}

const (
	likeKeyPrefix             = "like:v2:"
	likeEventSequenceKey      = likeKeyPrefix + "event_sequence"
	likeEventSequenceReadyKey = likeKeyPrefix + "event_sequence:ready"
	likeEventSequenceLockKey  = likeKeyPrefix + "event_sequence:lock"
	likeRankingKey            = likeKeyPrefix + "ranking"
	likeRankingReadyKey       = likeKeyPrefix + "ranking:ready"
	likeRankingLockKey        = likeKeyPrefix + "ranking:lock"
	likeOutboxList            = likeKeyPrefix + "outbox"
	likeOutboxProcessingList  = likeKeyPrefix + "outbox:processing"
	likeDirtyArticlesKey      = likeKeyPrefix + "dirty_articles"
	likePendingArticlesKey    = likeKeyPrefix + "pending_articles"
)

var (
	ErrLikeCacheWarming     = errors.New("like cache is warming")
	ErrLikeCacheUnavailable = errors.New("like cache is unavailable")
)

type LikeRank struct {
	ArticleID uint
	Likes     int64
}

func likeReadyKey(articleID uint) string {
	return likeKeyPrefix + "ready:" + strconv.FormatUint(uint64(articleID), 10)
}

func likeUsersKey(articleID uint) string {
	return likeKeyPrefix + "users:" + strconv.FormatUint(uint64(articleID), 10)
}

func likeCountKey(articleID uint) string {
	return likeKeyPrefix + "count:" + strconv.FormatUint(uint64(articleID), 10)
}

func likeCacheLockKey(articleID uint) string {
	return likeKeyPrefix + "init_lock:" + strconv.FormatUint(uint64(articleID), 10)
}

func likePendingEventsKey(articleID uint) string {
	return likeKeyPrefix + "pending_events:" + strconv.FormatUint(uint64(articleID), 10)
}

// InitLikeCacheInfrastructure initializes the global Redis projections before
// handlers begin accepting likes. It is safe to call again after a Redis restart.
func InitLikeCacheInfrastructure() {
	if global.RedisDB == nil || global.Db == nil {
		return
	}
	if err := ensureLikeEventSequence(); err != nil {
		logrus.Warnf("initialize like event sequence failed: %v", err)
	}
	if err := ensureLikeRanking(); err != nil {
		logrus.Warnf("initialize like ranking failed: %v", err)
	}
}

// ToggleLikeAsync accepts a like on Redis' single-threaded path. The same Lua
// script changes user state, count, ranking and the Redis List outbox, so a
// successful HTTP response always has an event waiting for RabbitMQ. Redis AOF
// is required in production to make that outbox crash-durable.
func ToggleLikeAsync(articleID, userID uint, direction int) (int64, error) {
	if articleID == 0 || userID == 0 || (direction != 1 && direction != -1) {
		return 0, errors.New("invalid like request")
	}
	// Redis is the fast-path source of truth. A deployment without Redis retains
	// the synchronous database fallback for availability, but does not claim the
	// high-concurrency asynchronous path.
	if global.RedisDB == nil {
		if LikeRedisConfigured() {
			return 0, ErrLikeCacheUnavailable
		}
		return ToggleLikeInDB(articleID, userID, direction)
	}

	if err := ensureLikeEventSequence(); err != nil {
		return 0, err
	}
	if err := ensureLikeCache(articleID); err != nil {
		return 0, err
	}

	article := strconv.FormatUint(uint64(articleID), 10)
	result, err := global.RedisDB.Eval(likeToggleScript,
		[]string{
			likeReadyKey(articleID),
			likeUsersKey(articleID),
			likeCountKey(articleID),
			likeRankingKey,
			likeOutboxList,
			likeEventSequenceKey,
			likeEventSequenceReadyKey,
			likeDirtyArticlesKey,
			likeRankingReadyKey,
			likePendingEventsKey(articleID),
			likePendingArticlesKey,
		},
		article,
		strconv.FormatUint(uint64(userID), 10),
		strconv.Itoa(direction),
	).Result()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) < 2 {
		return 0, fmt.Errorf("%w: unexpected lua result %v", ErrLikeCacheUnavailable, result)
	}
	status := toInt64(values[0])
	count := toInt64(values[1])
	switch status {
	case 1:
		return count, nil
	case 0:
		if direction == 1 {
			return count, ErrAlreadyLiked
		}
		return count, ErrNotLiked
	case -2, -3:
		return 0, ErrLikeCacheWarming
	default:
		return 0, fmt.Errorf("%w: unexpected lua status %d", ErrLikeCacheUnavailable, status)
	}
}

const likeToggleScript = `
if redis.call('GET', KEYS[1]) ~= '1' then
  return {-2, 0}
end
if redis.call('EXISTS', KEYS[2]) == 0 or redis.call('EXISTS', KEYS[3]) == 0 then
  return {-2, 0}
end
if redis.call('GET', KEYS[7]) ~= '1' or redis.call('EXISTS', KEYS[6]) == 0 then
  return {-3, 0}
end

local bit = redis.call('GETBIT', KEYS[2], ARGV[2])
local direction = tonumber(ARGV[3])
if direction == 1 and bit == 1 then
  local current = tonumber(redis.call('GET', KEYS[3]) or '0')
  return {0, current}
end
if direction == -1 and bit == 0 then
  local current = tonumber(redis.call('GET', KEYS[3]) or '0')
  return {0, current}
end

local count
if direction == 1 then
  redis.call('SETBIT', KEYS[2], ARGV[2], 1)
  count = redis.call('INCR', KEYS[3])
else
  redis.call('SETBIT', KEYS[2], ARGV[2], 0)
  count = redis.call('DECR', KEYS[3])
  if count < 0 then
    count = 0
    redis.call('SET', KEYS[3], '0')
  end
end

if count > 0 then
  redis.call('ZADD', KEYS[4], count, ARGV[1])
  redis.call('SET', KEYS[9], '1')
else
  redis.call('ZREM', KEYS[4], ARGV[1])
  redis.call('SET', KEYS[9], 'empty')
end

local version = redis.call('INCR', KEYS[6])
local eventId = 'redis-' .. version
redis.call('LPUSH', KEYS[5], ARGV[1] .. '|' .. ARGV[2] .. '|' .. ARGV[3] .. '|' .. version)
redis.call('SADD', KEYS[10], eventId)
redis.call('SADD', KEYS[11], ARGV[1])
redis.call('HSET', KEYS[8], ARGV[1], version)
return {1, count, version}
`

// GetLikeState reads the Redis projection used by the asynchronous write path.
// A configured-but-unavailable Redis instance must surface as unavailable,
// rather than expose a potentially stale MySQL projection while accepted
// outbox events are still pending. When Redis is deliberately not configured,
// writes use the synchronous database fallback and database reads are safe.
func GetLikeState(articleID, userID uint) (int64, bool, error) {
	if articleID == 0 {
		return 0, false, ErrLikeArticleNotFound
	}
	if global.RedisDB == nil {
		if LikeRedisConfigured() {
			return 0, false, ErrLikeCacheUnavailable
		}
		return getLikeStateFromDB(articleID, userID)
	}
	if err := ensureLikeCache(articleID); err != nil {
		return 0, false, err
	}

	// Read the ready marker, count and bitmap in one Redis operation. This
	// prevents a key eviction between separate GET/GETBIT calls from being
	// mistaken for a genuine zero-like state.
	result, err := global.RedisDB.Eval(`
if redis.call('GET', KEYS[1]) ~= '1' then
  return {-2, 0, 0}
end
if redis.call('EXISTS', KEYS[2]) == 0 or redis.call('EXISTS', KEYS[3]) == 0 then
  return {-2, 0, 0}
end
return {1, tonumber(redis.call('GET', KEYS[3])), redis.call('GETBIT', KEYS[2], ARGV[1])}
`, []string{likeReadyKey(articleID), likeUsersKey(articleID), likeCountKey(articleID)}, userID).Result()
	if err != nil {
		return 0, false, fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	values, ok := result.([]interface{})
	if !ok || len(values) != 3 {
		return 0, false, fmt.Errorf("%w: unexpected redis read result", ErrLikeCacheUnavailable)
	}
	if toInt64(values[0]) != 1 {
		// A cache key disappeared after ensureLikeCache completed. Treat this as
		// a short retry rather than returning an older database state.
		return 0, false, ErrLikeCacheWarming
	}
	return toInt64(values[1]), toInt64(values[2]) == 1, nil
}

func getLikeStateFromDB(articleID, userID uint) (int64, bool, error) {
	if global.Db == nil {
		return 0, false, ErrLikeCacheUnavailable
	}
	var article models.Article
	if err := global.Db.First(&article, articleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, false, ErrLikeArticleNotFound
		}
		return 0, false, err
	}
	var count int64
	if err := global.Db.Model(&models.Like{}).Where("article_id = ?", articleID).Count(&count).Error; err != nil {
		return 0, false, err
	}
	if userID == 0 {
		return count, false, nil
	}
	var liked int64
	if err := global.Db.Model(&models.Like{}).Where("article_id = ? AND user_id = ?", articleID, userID).Count(&liked).Error; err != nil {
		return 0, false, err
	}
	return count, liked > 0, nil
}

// GetHotLikeRanks reads the Redis real-time sorted-set projection. In the
// asynchronous mode an unavailable Redis projection cannot safely fall back to
// MySQL because the outbox may contain newer, accepted events.
func GetHotLikeRanks(limit int) ([]LikeRank, error) {
	if limit <= 0 {
		return []LikeRank{}, nil
	}
	if global.RedisDB == nil {
		return nil, ErrLikeCacheUnavailable
	}
	if err := ensureLikeRanking(); err != nil {
		return nil, err
	}
	result, err := global.RedisDB.Eval(`
local ready = redis.call('GET', KEYS[1])
if ready == 'empty' then
  return {1}
end
if ready ~= '1' or redis.call('EXISTS', KEYS[2]) == 0 then
  return {-2}
end
local rows = redis.call('ZREVRANGE', KEYS[2], 0, tonumber(ARGV[1]) - 1, 'WITHSCORES')
table.insert(rows, 1, 1)
return rows
`, []string{likeRankingReadyKey, likeRankingKey}, limit).Result()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	values, ok := result.([]interface{})
	if !ok || len(values) == 0 {
		return nil, fmt.Errorf("%w: unexpected ranking result", ErrLikeCacheUnavailable)
	}
	if toInt64(values[0]) != 1 {
		return nil, ErrLikeCacheWarming
	}
	ranks := make([]LikeRank, 0, (len(values)-1)/2)
	for i := 1; i+1 < len(values); i += 2 {
		id, err := strconv.ParseUint(fmt.Sprint(values[i]), 10, 64)
		if err != nil || id == 0 {
			continue
		}
		ranks = append(ranks, LikeRank{ArticleID: uint(id), Likes: toInt64(values[i+1])})
	}
	return ranks, nil
}

// InvalidateLikeCache is called after an article is deleted so an old Redis
// bitmap can never accept a like for a soft-deleted article.
func InvalidateLikeCache(articleID uint) {
	if global.RedisDB == nil || articleID == 0 {
		return
	}
	article := strconv.FormatUint(uint64(articleID), 10)
	_, _ = global.RedisDB.Pipelined(func(pipe redis.Pipeliner) error {
		pipe.Del(likeReadyKey(articleID), likeUsersKey(articleID), likeCountKey(articleID), likePendingEventsKey(articleID))
		pipe.ZRem(likeRankingKey, article)
		pipe.HDel(likeDirtyArticlesKey, article)
		pipe.SRem(likePendingArticlesKey, article)
		return nil
	})
}

// markLikeEventPersisted removes an event from the per-article pending set
// only after MySQL has committed it. This lets a lost Redis bitmap safely
// decide whether it may rebuild from the database projection.
func markLikeEventPersisted(msg LikeMessage) error {
	if global.RedisDB == nil || msg.EventID == "" {
		return ErrLikeCacheUnavailable
	}
	articleID, err := strconv.ParseUint(msg.ArticleID, 10, 64)
	if err != nil || articleID == 0 {
		return fmt.Errorf("%w: %v", ErrInvalidLikeMessage, err)
	}
	article := strconv.FormatUint(articleID, 10)
	const script = `
redis.call('SREM', KEYS[1], ARGV[1])
if redis.call('SCARD', KEYS[1]) == 0 then
  redis.call('DEL', KEYS[1])
  redis.call('SREM', KEYS[2], ARGV[2])
end
return 1
`
	if err := global.RedisDB.Eval(script, []string{likePendingEventsKey(uint(articleID)), likePendingArticlesKey}, msg.EventID, article).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	return nil
}

func ensureLikeEventSequence() error {
	if global.RedisDB == nil || global.Db == nil {
		return ErrLikeCacheUnavailable
	}
	ready, err := global.RedisDB.Get(likeEventSequenceReadyKey).Result()
	if err == nil && ready == "1" {
		exists, existsErr := global.RedisDB.Exists(likeEventSequenceKey).Result()
		if existsErr != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, existsErr)
		}
		if exists == 1 {
			return nil
		}
		// A partial Redis eviction must not let INCR restart at one while MySQL
		// already contains larger versions. The Lua script also checks this key.
		if err := global.RedisDB.Del(likeEventSequenceReadyKey).Err(); err != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
		}
	}
	if err != nil && err != redis.Nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}

	owner, acquired, err := acquireLikeLock(likeEventSequenceLockKey)
	if err != nil {
		return err
	}
	if !acquired {
		if waitForLikeReady(likeEventSequenceReadyKey) {
			return nil
		}
		return ErrLikeCacheWarming
	}
	defer releaseLikeLock(likeEventSequenceLockKey, owner)

	var maxVersion int64
	if err := global.Db.Unscoped().Model(&models.Like{}).Select("COALESCE(MAX(state_version), 0)").Scan(&maxVersion).Error; err != nil {
		return err
	}
	const script = `
local raw = redis.call('GET', KEYS[1])
local current = tonumber(raw or '0')
local minimum = tonumber(ARGV[1])
if not raw or current < minimum then
  redis.call('SET', KEYS[1], minimum)
end
redis.call('SET', KEYS[2], '1')
return 1
`
	if err := global.RedisDB.Eval(script, []string{likeEventSequenceKey, likeEventSequenceReadyKey}, maxVersion).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	return nil
}

func ensureLikeCache(articleID uint) error {
	if global.RedisDB == nil || global.Db == nil {
		return ErrLikeCacheUnavailable
	}
	readyKey := likeReadyKey(articleID)
	ready, err := global.RedisDB.Get(readyKey).Result()
	if err == nil && ready == "1" {
		exists, existsErr := global.RedisDB.Exists(likeUsersKey(articleID), likeCountKey(articleID)).Result()
		if existsErr != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, existsErr)
		}
		if exists == 2 {
			return nil
		}
		// Redis is the fast-path source of truth while events are in flight. Do
		// not rebuild a lost bitmap from an older MySQL projection in that case.
		pending, pendingErr := global.RedisDB.SCard(likePendingEventsKey(articleID)).Result()
		if pendingErr != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, pendingErr)
		}
		if pending > 0 {
			return ErrLikeCacheWarming
		}
		if err := global.RedisDB.Del(readyKey).Err(); err != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
		}
	}
	if err != nil && err != redis.Nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	if pending, pendingErr := global.RedisDB.SCard(likePendingEventsKey(articleID)).Result(); pendingErr != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, pendingErr)
	} else if pending > 0 {
		return ErrLikeCacheWarming
	}

	lockKey := likeCacheLockKey(articleID)
	owner, acquired, err := acquireLikeLock(lockKey)
	if err != nil {
		return err
	}
	if !acquired {
		if waitForLikeReady(readyKey) {
			return nil
		}
		return ErrLikeCacheWarming
	}
	defer releaseLikeLock(lockKey, owner)

	var article models.Article
	if err := global.Db.Select("id").First(&article, articleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrLikeArticleNotFound
		}
		return err
	}

	var likes []models.Like
	if err := global.Db.Unscoped().Select("user_id", "deleted_at").Where("article_id = ?", articleID).Find(&likes).Error; err != nil {
		return err
	}
	active := int64(0)
	for _, like := range likes {
		if !like.DeletedAt.Valid {
			active++
		}
	}

	usersKey := likeUsersKey(articleID)
	countKey := likeCountKey(articleID)
	tempSuffix := ":building:" + owner
	tempUsersKey := usersKey + tempSuffix
	tempCountKey := countKey + tempSuffix
	if _, err := global.RedisDB.Pipelined(func(pipe redis.Pipeliner) error {
		pipe.Del(tempUsersKey, tempCountKey)
		// User ID 0 is invalid, so reserve its bit as a non-zero sentinel. This
		// keeps an otherwise empty bitmap key present for the Lua safety check.
		pipe.SetBit(tempUsersKey, 0, 1)
		for _, like := range likes {
			if !like.DeletedAt.Valid {
				pipe.SetBit(tempUsersKey, int64(like.UserID), 1)
			}
		}
		pipe.Set(tempCountKey, strconv.FormatInt(active, 10), 0)
		return nil
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	if _, err := global.RedisDB.Pipelined(func(pipe redis.Pipeliner) error {
		pipe.Rename(tempUsersKey, usersKey)
		pipe.Rename(tempCountKey, countKey)
		pipe.Set(readyKey, "1", 0)
		return nil
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	return nil
}

func ensureLikeRanking() error {
	if global.RedisDB == nil || global.Db == nil {
		return ErrLikeCacheUnavailable
	}
	ready, err := global.RedisDB.Get(likeRankingReadyKey).Result()
	if err == nil && ready == "empty" {
		// The toggle Lua script changes this marker together with the ZSet. An
		// explicitly empty projection therefore remains valid even while the
		// final unlike event is waiting for MySQL.
		return nil
	}
	if err == nil && ready == "1" {
		exists, existsErr := global.RedisDB.Exists(likeRankingKey).Result()
		if existsErr != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, existsErr)
		}
		if exists == 1 {
			return nil
		}
		if err := global.RedisDB.Del(likeRankingReadyKey).Err(); err != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
		}
	}
	if err != nil && err != redis.Nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	if pending, pendingErr := global.RedisDB.SCard(likePendingArticlesKey).Result(); pendingErr != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, pendingErr)
	} else if pending > 0 {
		return ErrLikeCacheWarming
	}

	owner, acquired, err := acquireLikeLock(likeRankingLockKey)
	if err != nil {
		return err
	}
	if !acquired {
		if waitForLikeReady(likeRankingReadyKey) {
			return nil
		}
		return ErrLikeCacheWarming
	}
	defer releaseLikeLock(likeRankingLockKey, owner)

	type rankRow struct {
		ArticleID uint  `gorm:"column:article_id"`
		Likes     int64 `gorm:"column:likes"`
	}
	var rows []rankRow
	if err := global.Db.Raw(`SELECT l.article_id, COUNT(l.id) AS likes FROM likes l
		INNER JOIN articles a ON a.id = l.article_id AND a.deleted_at IS NULL
		WHERE l.deleted_at IS NULL GROUP BY l.article_id`).Scan(&rows).Error; err != nil {
		return err
	}

	if len(rows) == 0 {
		// Redis deletes an empty sorted set, so there is nothing to RENAME in
		// this branch. Remove an old projection before marking the empty one ready.
		if _, err := global.RedisDB.Pipelined(func(pipe redis.Pipeliner) error {
			pipe.Del(likeRankingKey)
			pipe.Set(likeRankingReadyKey, "empty", 0)
			return nil
		}); err != nil {
			return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
		}
		return nil
	}

	tempKey := likeRankingKey + ":building:" + owner
	if _, err := global.RedisDB.Pipelined(func(pipe redis.Pipeliner) error {
		pipe.Del(tempKey)
		for _, row := range rows {
			pipe.ZAdd(tempKey, redis.Z{Score: float64(row.Likes), Member: strconv.FormatUint(uint64(row.ArticleID), 10)})
		}
		return nil
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	if _, err := global.RedisDB.Pipelined(func(pipe redis.Pipeliner) error {
		pipe.Rename(tempKey, likeRankingKey)
		pipe.Set(likeRankingReadyKey, "1", 0)
		return nil
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	return nil
}

func acquireLikeLock(key string) (string, bool, error) {
	if global.RedisDB == nil {
		return "", false, ErrLikeCacheUnavailable
	}
	owner := strconv.FormatInt(time.Now().UnixNano(), 10)
	ok, err := global.RedisDB.SetNX(key, owner, 15*time.Second).Result()
	if err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrLikeCacheUnavailable, err)
	}
	return owner, ok, nil
}

func releaseLikeLock(key, owner string) {
	if global.RedisDB == nil || owner == "" {
		return
	}
	const script = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`
	_ = global.RedisDB.Eval(script, []string{key}, owner).Err()
}

func waitForLikeReady(key string) bool {
	if global.RedisDB == nil {
		return false
	}
	for i := 0; i < 50; i++ {
		ready, err := global.RedisDB.Get(key).Result()
		if err == nil && (ready == "1" || ready == "empty") {
			return true
		}
		if err != nil && err != redis.Nil {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(t), 10, 64)
		return n
	default:
		return 0
	}
}
