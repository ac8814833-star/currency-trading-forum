package middlewares

import (
	"back/utils"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization"})
			c.Abort()
			return
		}
		claims, err := utils.ParseAccessToken(token)
		if err != nil {
			if errors.Is(err, utils.ErrTokenStoreUnavailable) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service unavailable"})
				c.Abort()
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}
		c.Set("username", claims.Username)
		c.Set("user_id", claims.UserID)
		c.Set("jti", claims.JTI)
		c.Next()
	}
}
