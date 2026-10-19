package controllers

import (
	"back/global"
	"back/models"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateComment 创建评论
func CreateComment(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	articleID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || articleID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}
	var input struct {
		Content  string `json:"content" binding:"required"`
		ParentID *uint  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 服务端再校验一次内容非空
	if strings.TrimSpace(input.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "评论内容不能为空"})
		return
	}

	comment := models.Comment{ArticleID: uint(articleID), Content: strings.TrimSpace(input.Content), ParentID: input.ParentID, Username: username.(string)}

	// 查找用户ID
	var user models.User
	if err := global.Db.Where("username = ?", comment.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}
	comment.UserID = user.ID

	// 确保文章存在
	var article models.Article
	if err := global.Db.First(&article, comment.ArticleID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		return
	}

	// 如果parent_id不为nil，验证父评论存在
	if comment.ParentID != nil {
		var parentComment models.Comment
		if err := global.Db.First(&parentComment, *comment.ParentID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Parent comment not found"})
			return
		}
		// 确保父评论属于同一篇文章
		if parentComment.ArticleID != comment.ArticleID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Parent comment does not belong to this article"})
			return
		}
		if parentComment.ParentID != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "只能回复顶级评论"})
			return
		}
	}

	// 创建评论
	if err := global.Db.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, comment)
}

// GetComments 按顶级评论分页；回复跟随其父评论返回。
func GetComments(c *gin.Context) {
	articleID := c.Param("id")
	var article models.Article
	if err := global.Db.Select("id").First(&article, articleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		}
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	var total int64
	base := global.Db.Model(&models.Comment{}).Where("article_id = ? AND parent_id IS NULL", articleID)
	if err := base.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	var comments []models.Comment
	if err := global.Db.Where("article_id = ? AND parent_id IS NULL", articleID).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC, id ASC")
		}).
		Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": comments, "total": total, "page": page, "pageSize": pageSize})
}

// DeleteComment 删除评论（只能删除自己的评论）
func DeleteComment(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	commentID := c.Param("id")

	var comment models.Comment
	if err := global.Db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 检查是否是评论作者
	if comment.Username != username.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only delete your own comments"})
		return
	}

	// 删除整棵回复子树，防止留下不可见但计入总数的孤儿评论。
	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		ids := []uint{comment.ID}
		for i := 0; i < len(ids); i++ {
			var children []uint
			if err := tx.Model(&models.Comment{}).Where("parent_id = ?", ids[i]).Pluck("id", &children).Error; err != nil {
				return err
			}
			ids = append(ids, children...)
		}
		return tx.Where("id IN ?", ids).Delete(&models.Comment{}).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Comment deleted successfully"})
}

// GetCommentCount 获取文章的评论数量
func GetCommentCount(c *gin.Context) {
	articleID := c.Param("id")
	var article models.Article
	if err := global.Db.Select("id").First(&article, articleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		}
		return
	}

	var count int64
	if err := global.Db.Model(&models.Comment{}).
		Where("article_id = ? AND (parent_id IS NULL OR parent_id IN (SELECT id FROM (SELECT id FROM comments WHERE article_id = ? AND parent_id IS NULL AND deleted_at IS NULL) AS roots))", articleID, articleID).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}
