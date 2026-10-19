package controllers

import (
	"back/global"
	"back/models"
	"back/utils"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func handleLikeToggle(c *gin.Context, direction int) {
	articleID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || articleID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}
	uid := c.GetUint("user_id")
	if uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	count, err := utils.ToggleLikeAsync(uint(articleID), uid, direction)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLikeArticleNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		case errors.Is(err, utils.ErrAlreadyLiked):
			c.JSON(http.StatusBadRequest, gin.H{"error": "您已经点过赞了"})
		case errors.Is(err, utils.ErrNotLiked):
			c.JSON(http.StatusBadRequest, gin.H{"error": "您还没有点赞该文章"})
		case errors.Is(err, utils.ErrLikeCacheWarming), errors.Is(err, utils.ErrLikeCacheUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "点赞服务正在恢复，请稍后重试"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "点赞处理失败"})
		}
		return
	}
	// 点赞状态和排行榜已由 Redis Lua 脚本同步更新；文章详情中的冗余
	// likes_count 由批量汇总写库后再失效，避免这里提前删缓存又写回旧值。
	message := "点赞成功"
	if direction == -1 {
		message = "已取消点赞"
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "likes": count})
}

func LikeArticle(c *gin.Context)    { handleLikeToggle(c, 1) }
func DisLikeArticle(c *gin.Context) { handleLikeToggle(c, -1) }

func GetArticleLikes(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}
	count, liked, err := utils.GetLikeState(uint(id), c.GetUint("user_id"))
	if err != nil {
		if errors.Is(err, utils.ErrLikeArticleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "点赞状态暂不可用"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"likes": count, "is_liked": liked})
}

func GetHotArticles(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	type rankedRow struct {
		ID    uint  `gorm:"column:id"`
		Likes int64 `gorm:"column:likes"`
	}
	var ranked []rankedRow
	if ranks, err := utils.GetHotLikeRanks(limit); err == nil {
		ranked = make([]rankedRow, 0, len(ranks))
		for _, rank := range ranks {
			ranked = append(ranked, rankedRow{ID: rank.ArticleID, Likes: rank.Likes})
		}
	} else if global.RedisDB != nil || utils.LikeRedisConfigured() {
		// Redis is the source of the real-time ranking while accepted outbox
		// events may still be waiting for MySQL. Do not return a stale database
		// aggregate as if it were the current hot list.
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "热门榜暂不可用，请稍后重试"})
		return
	} else {
		// Redis was intentionally not configured, so likes use the synchronous
		// database path and the persisted relation is authoritative.
		if err := global.Db.Raw(`SELECT a.id, COUNT(l.id) AS likes FROM articles a
			LEFT JOIN likes l ON l.article_id = a.id AND l.deleted_at IS NULL
			WHERE a.deleted_at IS NULL GROUP BY a.id HAVING likes > 0
			ORDER BY likes DESC, a.id DESC LIMIT ?`, limit).Scan(&ranked).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	}
	if len(ranked) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}
	ids := make([]uint, 0, len(ranked))
	for _, row := range ranked {
		ids = append(ids, row.ID)
	}
	var articles []models.Article
	if err := global.Db.Where("id IN ?", ids).Find(&articles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	articleByID := make(map[uint]models.Article, len(articles))
	for _, article := range articles {
		articleByID[article.ID] = article
	}
	type hotArticle struct {
		models.Article
		Likes int64 `json:"likes"`
	}
	result := make([]hotArticle, 0, len(ranked))
	for _, row := range ranked {
		if article, ok := articleByID[row.ID]; ok {
			article.LikesCount = row.Likes
			result = append(result, hotArticle{Article: article, Likes: row.Likes})
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
