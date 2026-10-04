package handlers

import (
	"errors"
	"log"
	"net/http"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

// maxImportBytes caps an uploaded file. A 5000-row catalogue is well under a megabyte;
// anything larger is either not a product list or an attempt to make the server chew
// on something large.
const maxImportBytes = 4 << 20

// PreviewItemImport reads a CSV and says what is in it, writing nothing.
//
// POST /api/v1/items/import/preview
//
//	{"company_id": 1, "csv": "Name,SKU,Price\n...", "mapping": {"Rate": "price"}}
//
// Nothing is written here on purpose: a catalogue import is somebody's whole product
// list, and the first thing it should do is let them see what the app made of their
// file — which columns were recognised, which rows are broken, and which products they
// already have — while it is still free to change their mind.
func (h *itemHandler) PreviewItemImport(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req struct {
		CompanyID int                              `json:"company_id"`
		CSV       string                           `json:"csv"`
		Mapping   map[string]services.ImportColumn `json:"mapping"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	if len(req.CSV) > maxImportBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "That file is too large to import in one go. Split it and import in parts.",
		})
		return
	}

	owned, err := companyBelongsToUser(h.db.DB, int64(req.CompanyID), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	preview, err := services.ParseItemCSV(h.db.DB, req.CompanyID, req.CSV, req.Mapping)
	if err != nil {
		var input services.ImportInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Message})
			return
		}
		log.Println("failed to parse item import:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read that file"})
		return
	}

	c.JSON(http.StatusOK, preview)
}

// ApplyItemImport writes the rows the person chose to keep.
//
// POST /api/v1/items/import
//
//	{"company_id": 1, "rows": [{"line": 2, "action": "create", "item": {...}}]}
func (h *itemHandler) ApplyItemImport(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req struct {
		CompanyID int                     `json:"company_id"`
		Rows      []services.ImportAction `json:"rows"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	owned, err := companyBelongsToUser(h.db.DB, int64(req.CompanyID), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized company access"})
		return
	}

	result, err := services.ApplyItemImport(h.db.DB, req.CompanyID, userID, req.Rows)
	if err != nil {
		var input services.ImportInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Message})
			return
		}
		var rowErr services.ImportRowError
		if errors.As(err, &rowErr) {
			// Nothing was written: the whole import is one transaction. The row that
			// stopped it is named so it can be fixed in the file.
			c.JSON(http.StatusConflict, gin.H{
				"error":  "Nothing was imported. " + rowErr.Error(),
				"failed": rowErr.Result.Failed,
			})
			return
		}
		log.Println("failed to apply item import:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to import"})
		return
	}

	c.JSON(http.StatusOK, result)
}
