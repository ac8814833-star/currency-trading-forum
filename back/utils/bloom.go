package utils

import (
	"back/global"
	"back/models"
	"hash/fnv"
	"strconv"
)

// 使用 Redis Bitmap 实现的简单布隆过滤器，用于文章 ID 判断

const (
	articleBloomKey      = "bf:articles"
	articleBloomReadyKey = "bf:articles:ready"
	articleBloomBuildKey = "bf:articles:building"
	articleBloomSize     = 1_000_000 // 位图大小（bit），约 125KB
	bloomHashFuncs       = 3         // 使用 3 个 hash 函数
)

// InitBloomFilter 初始化布隆过滤器，将数据库中现有的文章 ID 预热进过滤器
func InitBloomFilter() {
	if global.Db == nil || global.RedisDB == nil {
		return
	}

	var articleIDs []uint
	// 只查询 ID，减少资源消耗
	if err := global.Db.Model(&models.Article{}).Pluck("id", &articleIDs).Error; err != nil {
		return
	}

	// 只有完整构建并切换后才允许据此拒绝请求。缺失/逐出的位图回源数据库。
	if err := global.RedisDB.Del(articleBloomReadyKey, articleBloomBuildKey).Err(); err != nil {
		return
	}
	if err := global.RedisDB.SetBit(articleBloomBuildKey, articleBloomSize, 1).Err(); err != nil {
		return
	}
	for _, id := range articleIDs {
		for _, pos := range bloomHashes(strconv.FormatUint(uint64(id), 10)) {
			if err := global.RedisDB.SetBit(articleBloomBuildKey, int64(pos), 1).Err(); err != nil {
				return
			}
		}
	}
	if err := global.RedisDB.Rename(articleBloomBuildKey, articleBloomKey).Err(); err != nil {
		return
	}
	_ = global.RedisDB.Set(articleBloomReadyKey, "1", 0).Err()
}

// 布隆过滤器内部使用的 hash 函数
func bloomHashes(value string) []uint64 {
	hashes := make([]uint64, bloomHashFuncs)
	for i := 0; i < bloomHashFuncs; i++ {
		h := fnv.New64a()
		// 不同种子：简单地在前面加上下标
		h.Write([]byte{byte(i)})
		h.Write([]byte(value))
		sum := h.Sum64()
		hashes[i] = sum % articleBloomSize
	}
	return hashes
}

// AddArticleToBloom 将文章 ID 加入布隆过滤器
func AddArticleToBloom(id string) {
	if global.RedisDB == nil {
		return
	}
	positions := bloomHashes(id)
	args := make([]interface{}, 0, len(positions))
	for _, pos := range positions {
		args = append(args, pos)
	}
	const script = `
if redis.call('EXISTS', KEYS[1]) == 0 or redis.call('EXISTS', KEYS[2]) == 0 then
  redis.call('DEL', KEYS[1])
  return 0
end
for i = 1, #ARGV do redis.call('SETBIT', KEYS[2], ARGV[i], 1) end
return 1`
	_ = global.RedisDB.Eval(script, []string{articleBloomReadyKey, articleBloomKey}, args...).Err()
}

// MightContainArticle 判断文章 ID 是否“可能存在”
// 返回 false：一定不存在，可以直接返回 404
// 返回 true：可能存在，需要继续查数据库
func MightContainArticle(id string) bool {
	if global.RedisDB == nil {
		// 没有 Redis 时，退化为总是“可能存在”，不影响功能
		return true
	}
	positions := bloomHashes(id)
	args := make([]interface{}, 0, len(positions))
	for _, pos := range positions {
		args = append(args, pos)
	}
	const script = `
if redis.call('EXISTS', KEYS[1]) == 0 or redis.call('EXISTS', KEYS[2]) == 0 then return 1 end
for i = 1, #ARGV do
  if redis.call('GETBIT', KEYS[2], ARGV[i]) == 0 then return 0 end
end
return 1`
	possible, err := global.RedisDB.Eval(script, []string{articleBloomReadyKey, articleBloomKey}, args...).Int()
	return err != nil || possible != 0
}
