package router

import (
	"net/http"

	"enterprise-pdf-ai/internal/api/handlers"
	"enterprise-pdf-ai/internal/api/middleware"
	"enterprise-pdf-ai/internal/configs"
	"github.com/gin-gonic/gin"
)

func New(cfg *configs.Config, documentHandler *handlers.DocumentHandler, chatHandler *handlers.ChatHandler) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RateLimiter(20, 40))

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "pong",
			"app":     cfg.App.Name,
		})
	})

	api := r.Group("/api/v1")
	api.Use(middleware.JWTAuth(cfg.Auth.JWTSecret))
	{
		api.POST("/documents", documentHandler.UploadDocument)
		api.POST("/chat", chatHandler.Chat)
	}

	return r
}
