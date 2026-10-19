package controllers

import (
	"back/global"
	"back/models"
	"back/utils"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func tokenResponse(ctx *gin.Context, status int, pair *utils.TokenPair, message string) {
	body := gin.H{
		"accessToken":  pair.AccessToken,
		"refreshToken": pair.RefreshToken,
		// 兼容旧前端字段
		"token": pair.AccessToken,
	}
	if message != "" {
		body["message"] = message
	}
	ctx.JSON(status, body)
}

func Register(ctx *gin.Context) {
	var user models.User
	if err := ctx.ShouldBind(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existingUser models.User
	if err := global.Db.Where("username = ?", user.Username).First(&existingUser).Error; err == nil {
		ctx.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		return
	}

	hashedPwd, err := utils.HashPassword(user.Password)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	user.Password = hashedPwd

	var pair *utils.TokenPair
	err = global.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		var tokenErr error
		pair, tokenErr = utils.GenerateTokenPair(user.Username, user.ID)
		return tokenErr
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, utils.ErrTokenStoreUnavailable) {
			status = http.StatusServiceUnavailable
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}

	tokenResponse(ctx, http.StatusCreated, pair, "Registration successful")
}

func GetCaptcha(c *gin.Context) {
	id, b64s, err := utils.CreateCaptcha()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Captcha service unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"captchaId":    id,
		"captchaImage": b64s,
	})
}

func Login(ctx *gin.Context) {
	var input struct {
		Username     string `json:"username" binding:"required"`
		Password     string `json:"password" binding:"required"`
		CaptchaId    string `json:"captchaId" binding:"required"`
		CaptchaValue string `json:"captchaValue" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validCaptcha, err := utils.VerifyCaptcha(input.CaptchaId, input.CaptchaValue)
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "Captcha service unavailable"})
		return
	}
	if !validCaptcha {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid captcha"})
		return
	}

	var user models.User
	if err := global.Db.Where("username = ?", input.Username).First(&user).Error; err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "wrong credentials"})
		return
	}
	if !utils.CheckPassword(input.Password, user.Password) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "wrong credentials"})
		return
	}

	pair, err := utils.GenerateTokenPair(user.Username, user.ID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, utils.ErrTokenStoreUnavailable) {
			status = http.StatusServiceUnavailable
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	tokenResponse(ctx, http.StatusOK, pair, "")
}

// Refresh 使用 refresh token 换取新的 access + refresh
func Refresh(ctx *gin.Context) {
	var input struct {
		RefreshToken string `json:"refreshToken" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pair, err := utils.RefreshTokens(input.RefreshToken)
	if err != nil {
		if errors.Is(err, utils.ErrTokenStoreUnavailable) {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "Token service unavailable"})
			return
		}
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
		return
	}
	tokenResponse(ctx, http.StatusOK, pair, "")
}

// Logout 登出：拉黑当前 access token，吊销 refresh token
func Logout(ctx *gin.Context) {
	accessJTI, _ := ctx.Get("jti")
	jtiStr, _ := accessJTI.(string)

	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = ctx.ShouldBindJSON(&input)

	if err := utils.LogoutTokens(jtiStr, input.RefreshToken); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, utils.ErrTokenStoreUnavailable) {
			status = http.StatusServiceUnavailable
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// GetCurrentUser 获取当前登录用户信息
func GetCurrentUser(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var user models.User
	if err := global.Db.Where("username = ?", username).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	user.Password = ""
	c.JSON(http.StatusOK, gin.H{
		"username": user.Username,
		"id":       user.ID,
	})
}
