# Redis 功能使用示例

## 1. 缓存操作

### 缓存文章列表
```go
import "back/utils"

// 设置缓存（10分钟过期）
articles := []models.Article{...}
err := utils.SetCache("articles", articles, 10*time.Minute)

// 获取缓存
var articles []models.Article
err := utils.GetCache("articles", &articles)

// 删除缓存
err := utils.DeleteCache("articles")
```

## 2. 计数器操作

### 文章点赞计数
```go
// 增加点赞数
count, err := utils.IncrementCounter("article:123:likes")

// 获取点赞数
count, err := utils.GetCounter("article:123:likes")
```

## 3. 字符串操作

### 存储简单字符串
```go
// 设置带过期时间的字符串
err := utils.SetWithExpiration("session:user123", "active", 30*time.Minute)

// 获取字符串
value, err := utils.GetString("session:user123")

// 检查键是否存在
exists, err := utils.Exists("session:user123")
```

## 4. 哈希操作（适合存储对象）

### 存储用户信息
```go
// 设置哈希字段
user := models.User{Username: "test", ID: 1}
err := utils.SetHash("user:1", "profile", user)

// 获取哈希字段
var user models.User
err := utils.GetHash("user:1", "profile", &user)

// 删除哈希字段
err := utils.DeleteHash("user:1", "profile")
```

## 5. 集合操作（适合去重、成员检查）

### 记录已读文章
```go
// 添加已读文章ID
err := utils.AddToSet("user:1:read", "article:123")

// 检查是否已读
isRead, err := utils.IsMemberOfSet("user:1:read", "article:123")

// 移除已读记录
err := utils.RemoveFromSet("user:1:read", "article:123")
```

## 6. 过期时间管理

### 设置键的过期时间
```go
// 为已存在的键设置过期时间
err := utils.SetExpiration("article:123", 1*time.Hour)
```

## 实际应用场景示例

### 场景1: 用户会话管理
```go
// 登录时设置会话
sessionKey := "session:" + username
err := utils.SetWithExpiration(sessionKey, token, 24*time.Hour)

// 检查会话是否有效
exists, err := utils.Exists(sessionKey)
```

### 场景2: 文章浏览量统计
```go
// 增加浏览量
count, err := utils.IncrementCounter("article:123:views")

// 获取浏览量
views, err := utils.GetCounter("article:123:views")
```

### 场景3: 热门文章排行榜
```go
// 使用有序集合（需要直接使用 global.RedisDB）
// 增加文章分数
global.RedisDB.ZIncrBy("hot:articles", 1, "article:123")

// 获取排行榜
results, err := global.RedisDB.ZRevRangeWithScores("hot:articles", 0, 9).Result()
```

### 场景4: 限流（Rate Limiting）
```go
// 记录用户请求次数
key := "rate_limit:user:123:" + time.Now().Format("2006010215")
count, err := utils.IncrementCounter(key)
if count == 1 {
    // 第一次请求，设置过期时间
    utils.SetExpiration(key, 1*time.Hour)
}
if count > 100 {
    // 超过限制
    return errors.New("rate limit exceeded")
}
```

