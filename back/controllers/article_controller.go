package controllers

import (
	"back/global"
	"back/models"
	"back/utils"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var cacheKey = "articles"

// SearchArticles 关键词搜索文章（ES 优先，MySQL 降级）
func SearchArticles(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "搜索关键词不能为空"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	result, err := utils.SearchArticles(q, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func CreateArticle(c *gin.Context) {
	// 获取当前登录用户
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var input struct {
		Title   string `json:"title" binding:"required"`
		Content string `json:"content" binding:"required"`
		Preview string `json:"preview" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	input.Preview = strings.TrimSpace(input.Preview)
	if input.Title == "" || input.Content == "" || input.Preview == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title, preview and content are required"})
		return
	}
	article := models.Article{Title: input.Title, Content: input.Content, Preview: input.Preview}
	// 设置作者信息
	article.Username = username.(string)

	// 查找用户ID
	var user models.User
	if err := global.Db.Where("username = ?", article.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}
	article.UserID = user.ID
	article.RagStatus = utils.RAGStatusPending
	article.RagVersion = 1
	article.RagError = ""
	article.RagIndexedAt = nil
	article.ContentHash = utils.BuildArticleIndexHash(article)

	if err := global.Db.Create(&article).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 关键词检索与 RAG 索引独立：即使向量化或模型服务暂时不可用，新文章仍可被 ES 搜索到。
	go func(a models.Article) {
		_ = utils.IndexArticleToES(&a)
	}(article)

	// 将新文章 ID 加入布隆过滤器
	utils.AddArticleToBloom(strconv.FormatUint(uint64(article.ID), 10))
	// MQ 发布失败时保持 pending，交给 sync worker 兜底重试，不要立刻标 failed
	if err := utils.AsyncIndexArticleByID(article.ID, article.RagVersion); err != nil {
		article.RagStatus = utils.RAGStatusPending
		article.RagError = "index queue unavailable, waiting for retry"
	}

	if global.RedisDB != nil {
		_ = global.RedisDB.Del(cacheKey).Err()
	}

	c.JSON(http.StatusCreated, article)

}

func GetArticles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	var total int64
	if err := global.Db.Model(&models.Article{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	var articles []models.Article
	if err := global.Db.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&articles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": articles, "total": total, "page": page, "pageSize": pageSize})
}

func GetArticlesById(c *gin.Context) {
	id := c.Param("id")
	articleCacheKey := "article:detail:" + id

	// 先查缓存
	if global.RedisDB != nil {
		cachedData, err := global.RedisDB.Get(articleCacheKey).Result()
		if err == nil {
			var article models.Article
			if err := json.Unmarshal([]byte(cachedData), &article); err == nil {
				c.JSON(http.StatusOK, article)
				return
			}
		}
	}

	// 缓存没中，查布隆过滤器
	// 如果布隆过滤器说不存在，那一定不存在（防止缓存穿透）
	if !utils.MightContainArticle(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		return
	}

	//数据库 (旁路缓存回源)
	var article models.Article
	if err := global.Db.Where("id = ?", id).First(&article).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 数据库也没有，可以考虑缓存一个空值（进一步防止缓存穿透，此处已有布隆过滤器，双重保障）
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 写入缓存，设置过期时间（如 1 小时）
	articleJSON, _ := json.Marshal(article)
	if global.RedisDB != nil {
		_ = global.RedisDB.Set(articleCacheKey, articleJSON, time.Hour).Err()
	}

	c.JSON(http.StatusOK, article)
}

// UpdateArticle 更新文章（只能更新自己的文章）
func UpdateArticle(c *gin.Context) {
	// 获取当前登录用户
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	articleID := c.Param("id")
	articleDetailCacheKey := "article:detail:" + articleID
	// 查找文章
	var article models.Article
	if err := global.Db.Where("id = ?", articleID).First(&article).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 权限校验：必须是作者
	if article.Username != username.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only edit your own articles"})
		return
	}

	// 绑定更新数据
	var updateData struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Preview string `json:"preview"`
	}
	if err := c.ShouldBind(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新文章（只更新允许的字段）
	updates := make(map[string]interface{})
	updatedArticle := article
	if updateData.Title != "" {
		updates["title"] = updateData.Title
		updatedArticle.Title = updateData.Title
	}
	if updateData.Content != "" {
		updates["content"] = updateData.Content
		updatedArticle.Content = updateData.Content
	}
	if updateData.Preview != "" {
		updates["preview"] = updateData.Preview
		updatedArticle.Preview = updateData.Preview
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, article)
		return
	}

	newContentHash := utils.BuildArticleIndexHash(updatedArticle)
	shouldReindex := article.ContentHash == "" || article.ContentHash != newContentHash
	if shouldReindex {
		nextVersion := article.RagVersion
		if nextVersion <= 0 {
			nextVersion = 1
		} else if article.ContentHash != "" && article.ContentHash != newContentHash {
			nextVersion++
		}
		updates["rag_status"] = utils.RAGStatusPending
		updates["rag_version"] = nextVersion
		updates["rag_error"] = ""
		updates["rag_indexed_at"] = nil
		updates["content_hash"] = newContentHash
	}

	if err := global.Db.Model(&article).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 清除缓存
	if global.RedisDB != nil {
		_ = global.RedisDB.Del(cacheKey, articleDetailCacheKey).Err()
	}

	// 重新查询更新后的文章
	global.Db.First(&article, articleID)
	// 不把关键词索引的可用性绑在 RAG 成功与否上。
	go func(a models.Article) {
		_ = utils.IndexArticleToES(&a)
	}(article)
	if shouldReindex {
		// MQ 失败保持 pending，由 sync worker 重试
		if err := utils.AsyncIndexArticleByID(article.ID, article.RagVersion); err != nil {
			article.RagStatus = utils.RAGStatusPending
			article.RagError = "index queue unavailable, waiting for retry"
		}
	}
	c.JSON(http.StatusOK, article)
}

// DeleteArticle 删除文章（只能删除自己的文章）
func DeleteArticle(c *gin.Context) {
	// 获取当前登录用户
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	articleID := c.Param("id")
	articleDetailCacheKey := "article:detail:" + articleID

	// 查找文章
	var article models.Article
	if err := global.Db.Where("id = ?", articleID).First(&article).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 权限校验：必须是作者
	if article.Username != username.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only delete your own articles"})
		return
	}

	// 软删除文章（GORM的Delete是软删除）
	if err := global.Db.Delete(&article).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := utils.DeleteArticleChunks(article.ID); err != nil {
		// 向量清理失败交给后续同步任务兜底，不阻塞主删除流程
	}

	// 从 Elasticsearch 中删除
	go utils.DeleteArticleFromES(article.ID)

	// 清除缓存
	utils.InvalidateLikeCache(article.ID)
	if global.RedisDB != nil {
		if err := global.RedisDB.Del(cacheKey).Err(); err != nil {
			// 缓存删除失败不影响主流程
		}
		if err := global.RedisDB.Del(articleDetailCacheKey).Err(); err != nil {
			// 缓存删除失败不影响主流程
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Article deleted successfully"})
}
