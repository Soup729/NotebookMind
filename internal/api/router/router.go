package router

import (
	"net/http"

	"enterprise-pdf-ai/internal/api/handlers"
	"enterprise-pdf-ai/internal/api/middleware"
	"enterprise-pdf-ai/internal/configs"
	"github.com/gin-gonic/gin"
)

func New(
	cfg *configs.Config,
	authHandler *handlers.AuthHandler,
	documentHandler *handlers.DocumentHandler,
	chatHandler *handlers.ChatHandler,
	dashboardHandler *handlers.DashboardHandler,
	searchHandler *handlers.SearchHandler,
	usageHandler *handlers.UsageHandler,
	notebookHandler *handlers.NotebookHandler,
	noteHandler *handlers.NoteHandler,
) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())
	r.Use(middleware.RateLimiter(20, 40))

	api := r.Group("/api/v1")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"message": "pong",
				"app":     cfg.App.Name,
			})
		})

		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
	}

	protected := api.Group("")
	protected.Use(middleware.JWTAuth(cfg.Auth.JWTSecret))
	{
		protected.GET("/me", authHandler.Me)

		protected.GET("/dashboard/overview", dashboardHandler.Overview)
		protected.GET("/usage/summary", usageHandler.Summary)

		protected.GET("/documents", documentHandler.ListDocuments)
		protected.POST("/documents", documentHandler.UploadDocument)
		protected.GET("/documents/:id", documentHandler.GetDocument)
		protected.DELETE("/documents/:id", documentHandler.DeleteDocument)

		protected.GET("/chat/sessions", chatHandler.ListSessions)
		protected.POST("/chat/sessions", chatHandler.CreateSession)
		protected.GET("/chat/sessions/:id/messages", chatHandler.ListMessages)
		protected.POST("/chat/sessions/:id/messages", chatHandler.SendMessage)

		protected.GET("/search", searchHandler.Search)

		// NotebookLM Routes
		protected.GET("/notebooks", notebookHandler.ListNotebooks)
		protected.POST("/notebooks", notebookHandler.CreateNotebook)
		protected.GET("/notebooks/:id", notebookHandler.GetNotebook)
		protected.PUT("/notebooks/:id", notebookHandler.UpdateNotebook)
		protected.DELETE("/notebooks/:id", notebookHandler.DeleteNotebook)

		// Notebook Documents
		protected.POST("/notebooks/:id/documents", notebookHandler.AddDocument)
		protected.DELETE("/notebooks/:id/documents/:documentId", notebookHandler.RemoveDocument)
		protected.GET("/notebooks/:id/documents", notebookHandler.ListDocuments)

		// Notebook Document Guide (Summary & FAQ)
		protected.GET("/notebooks/:id/documents/:documentId/guide", notebookHandler.GetDocumentGuide)

		// Notebook Chat Sessions
		protected.GET("/notebooks/:id/sessions", notebookHandler.ListSessions)
		protected.POST("/notebooks/:id/sessions", notebookHandler.CreateSession)

		// Notebook Streaming Chat
		protected.POST("/notebooks/:id/sessions/:sessionId/chat", notebookHandler.StreamChat)

		// Notebook Search
		protected.POST("/notebooks/:id/search", notebookHandler.SearchNotebook)

		// Notes (Research Notes)
		protected.GET("/notes", noteHandler.List)
		protected.POST("/notes", noteHandler.Create)
		protected.GET("/notes/:id", noteHandler.Get)
		protected.PUT("/notes/:id", noteHandler.Update)
		protected.DELETE("/notes/:id", noteHandler.Delete)
		protected.POST("/notes/:id/pin", noteHandler.Pin)
		protected.POST("/notes/:id/tags", noteHandler.AddTag)
		protected.DELETE("/notes/:id/tags", noteHandler.RemoveTag)
		protected.GET("/notes/tags/search", noteHandler.SearchByTag)
	}

	return r
}
