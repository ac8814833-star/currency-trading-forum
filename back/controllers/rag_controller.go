package controllers

import (
	"back/global"
	"back/models"
	"back/utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func IndexArticleToRAG(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var article models.Article
	if err := global.Db.First(&article, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found"})
		return
	}
	if uid, ok := userID.(uint); !ok || article.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only index your own article"})
		return
	}

	if err := utils.IndexArticleByID(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "索引服务暂不可用，请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "article indexed",
		"article_id": uint(id),
	})
}

type RAGAskRequest struct {
	Question string `json:"question" binding:"required,max=500"`
	TopK     int    `json:"top_k"`
}

func AskRAG(c *gin.Context) {
	var req RAGAskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question is required"})
		return
	}
	if req.TopK < 0 || req.TopK > 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "top_k must be between 0 and 8"})
		return
	}

	hits, err := utils.RetrieveRAGHits(req.Question, req.TopK)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "检索服务暂不可用，请稍后重试"})
		return
	}

	answer, err := utils.GenerateRAGAnswer(req.Question, hits)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "问答模型暂不可用，请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"answer": answer,
		"hits":   hits,
	})
}
