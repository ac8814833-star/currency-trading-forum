package router

import (
	"back/controllers"
	"back/middlewares"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"os"
	"strings"
	"time"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	// 默认不信任 X-Forwarded-For。只有部署在明确的反向代理后才配置其 IP/CIDR。
	trusted := []string(nil)
	if raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); raw != "" {
		for _, entry := range strings.Split(raw, ",") {
			if proxy := strings.TrimSpace(entry); proxy != "" {
				trusted = append(trusted, proxy)
			}
		}
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		panic("invalid TRUSTED_PROXIES: " + err.Error())
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "DELETE", "PUT", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	auth := r.Group("/api/auth")
	{
		// 登录限流：同一 IP/用户 每分钟最多 10 次
		auth.POST("/login",
			middlewares.RateLimitMiddleware(middlewares.RateLimitConfig{
				KeyPrefix:      "rate:login",
				BucketCapacity: 10,          // 桶容量 10
				RefillRate:     10.0 / 60.0, // 每分钟补满 10 个
				BlockedMsg:     "登录请求过于频繁，请稍后再试",
				BlockedStatus:  429,
				Expire:         time.Hour,
			}),
			controllers.Login,
		)
		auth.POST("/register", controllers.Register)
		auth.POST("/refresh", controllers.Refresh)
		auth.GET("/captcha", controllers.GetCaptcha) //验证码
	}
	api := r.Group("/api")
	// 汇率兑换（公开，无需登录）
	api.GET("/currency/rates", controllers.GetCurrencyRates)
	api.GET("/currency/convert", controllers.ConvertCurrency)
	// 公开的评论查看接口
	api.GET("/articles/:id/comments", controllers.GetComments)
	api.GET("/articles/:id/comments/count", controllers.GetCommentCount)
	api.Use(middlewares.AuthMiddleware())
	{
		api.POST("/auth/logout", controllers.Logout)
		// 获取当前用户信息
		api.GET("/user/me", controllers.GetCurrentUser)
		// 文章与排行榜
		// 发文章限流：提升桶容量
		api.POST("/articles",
			middlewares.RateLimitMiddleware(middlewares.RateLimitConfig{
				KeyPrefix:      "rate:create_article_v2", // 更改前缀，强制刷新 Redis Key
				BucketCapacity: 20,
				RefillRate:     20.0 / 60.0,
				BlockedMsg:     "发文频率已达到上限，请 1 分钟后再试",
				BlockedStatus:  429,
				Expire:         time.Hour,
			}),
			controllers.CreateArticle,
		)
		api.GET("/articles", controllers.GetArticles)
		api.GET("/articles/hot", controllers.GetHotArticles)    // 热门文章排行榜（按点赞量）
		api.GET("/articles/search", controllers.SearchArticles) // ES 全文搜索（失败降级 MySQL）
		api.GET("/articles/:id", controllers.GetArticlesById)
		api.PUT("/articles/:id", controllers.UpdateArticle)    //编辑文章
		api.DELETE("/articles/:id", controllers.DeleteArticle) //删除文章
		// 点赞/取消点赞统一限流，避免通过交替请求绕过限流
		api.POST("/articles/:id/like",
			middlewares.RateLimitMiddleware(middlewares.RateLimitConfig{
				KeyPrefix:      "rate:like_action_v3",
				BucketCapacity: 10,
				RefillRate:     1.0, // 每秒恢复 1 个令牌
				BlockedMsg:     "操作过于频繁，请稍后再试",
				BlockedStatus:  429,
				Expire:         time.Hour,
			}),
			controllers.LikeArticle,
		)
		api.POST("/articles/:id/unlike",
			middlewares.RateLimitMiddleware(middlewares.RateLimitConfig{
				KeyPrefix:      "rate:like_action_v3",
				BucketCapacity: 10,
				RefillRate:     1.0,
				BlockedMsg:     "操作过于频繁，请稍后再试",
				BlockedStatus:  429,
				Expire:         time.Hour,
			}),
			controllers.DisLikeArticle,
		)
		api.GET("/articles/:id/like", controllers.GetArticleLikes)
		// 评论相关路由（需要登录）
		api.POST("/articles/:id/comments", controllers.CreateComment)
		api.DELETE("/comments/:id", controllers.DeleteComment)
		api.POST("/rag/index/:id",
			middlewares.RateLimitMiddleware(middlewares.RateLimitConfig{
				KeyPrefix:      "rate:rag_index",
				BucketCapacity: 3,
				RefillRate:     3.0 / 60.0,
				BlockedMsg:     "索引请求过于频繁，请稍后再试",
				BlockedStatus:  429,
				Expire:         time.Hour,
			}),
			controllers.IndexArticleToRAG,
		)
		api.POST("/rag/ask",
			middlewares.RateLimitMiddleware(middlewares.RateLimitConfig{
				KeyPrefix:      "rate:rag_ask",
				BucketCapacity: 5,
				RefillRate:     5.0 / 60.0,
				BlockedMsg:     "问答请求过于频繁，请稍后再试",
				BlockedStatus:  429,
				Expire:         time.Hour,
			}),
			controllers.AskRAG,
		)
	}
	return r
}
