package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type GSTReportHandler struct {
	db *sql.DB
}

func NewGSTReportHandler(db *sql.DB) *GSTReportHandler {
	return &GSTReportHandler{db: db}
}

// GET /api/v1/companies/:companyId/reports/gstr1?start=YYYY-MM-DD&end=YYYY-MM-DD
func (h *GSTReportHandler) GetGSTReport(c *gin.Context) {
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

	start := c.Query("start")
	end := c.Query("end")
	if start == "" || end == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start and end (YYYY-MM-DD) are required"})
		return
	}

	report, err := services.GenerateGSTReport(h.db, companyID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate report"})
		return
	}

	c.JSON(http.StatusOK, report)
}
