package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type AgingReportHandler struct {
	db *sql.DB
}

func NewAgingReportHandler(db *sql.DB) *AgingReportHandler {
	return &AgingReportHandler{db: db}
}

// GET /api/v1/companies/:companyId/reports/aging
func (h *AgingReportHandler) GetAgingReport(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("companyId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid companyId"})
		return
	}

	userID := c.GetInt("user_id")
	owned, err := companyBelongsToUser(h.db, companyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	report, err := services.GenerateAgingReport(h.db, companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate report"})
		return
	}

	c.JSON(http.StatusOK, report)
}
