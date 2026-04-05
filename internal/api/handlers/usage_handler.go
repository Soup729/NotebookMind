package handlers

import (
	"net/http"

	"NotebookAI/internal/service"
	"github.com/gin-gonic/gin"
)

type UsageHandler struct {
	dashboardService service.DashboardService
}

func NewUsageHandler(dashboardService service.DashboardService) *UsageHandler {
	return &UsageHandler{dashboardService: dashboardService}
}

func (h *UsageHandler) Summary(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	summary, err := h.dashboardService.GetUsageSummary(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load usage summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}
