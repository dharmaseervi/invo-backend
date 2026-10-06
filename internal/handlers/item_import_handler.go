package handlers

import (
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strings"

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
		CompanyID int    `json:"company_id"`
		CSV       string `json:"csv"`
		// An Excel workbook, base64 encoded. Every shop with a price list has it in
		// Excel, and "save as CSV, pick the right encoding" is where people give up.
		XLSX    string                           `json:"xlsx"`
		Mapping map[string]services.ImportColumn `json:"mapping"`
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

	content, err := importContent(req.CSV, req.XLSX)
	if err != nil {
		var input services.ImportInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"error": input.Message})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "That file could not be read."})
		return
	}

	preview, err := services.ParseItemCSV(h.db.DB, req.CompanyID, content, req.Mapping)
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

// importContent turns whichever of the two the caller sent into the text the parser
// reads.
//
// An Excel workbook is checked by its contents rather than trusted because of which
// field it arrived in: somebody who renames a CSV to .xlsx, or the reverse, should still
// get their catalogue imported rather than an error about a format they did not choose.
func importContent(csvText, xlsxBase64 string) (string, error) {
	if strings.TrimSpace(xlsxBase64) != "" {
		raw, err := base64.StdEncoding.DecodeString(xlsxBase64)
		if err != nil {
			return "", services.ImportInputError{Message: "That file could not be read."}
		}
		if len(raw) > maxImportBytes {
			return "", services.ImportInputError{
				Message: "That file is too large to import in one go. Split it and import in parts.",
			}
		}
		if services.LooksLikeXLSX(raw) {
			return services.XLSXToCSV(raw)
		}
		// Not a workbook after all — most likely a CSV that was renamed. Read it as
		// one rather than refusing over the extension.
		return string(raw), nil
	}

	// A CSV pasted or read as text. It may still be a workbook somebody dropped in
	// whole, which would arrive as unreadable bytes rather than as something to parse.
	if services.LooksLikeXLSX([]byte(csvText)) {
		return "", services.ImportInputError{
			Message: "That looks like an Excel file. Send it as a file rather than as text and it will import.",
		}
	}
	return csvText, nil
}
