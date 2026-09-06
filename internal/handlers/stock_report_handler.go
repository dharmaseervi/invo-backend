package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type StockReportHandler struct {
	db *sql.DB
}

func NewStockReportHandler(db *sql.DB) *StockReportHandler {
	return &StockReportHandler{db: db}
}

// GET /api/v1/companies/:companyId/reports/stock
func (h *StockReportHandler) GetStockReport(c *gin.Context) {
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

	report, err := services.GenerateStockReport(h.db, companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate report"})
		return
	}

	c.JSON(http.StatusOK, report)
}
