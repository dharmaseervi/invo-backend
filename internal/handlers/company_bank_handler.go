package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"invo-server/internal/models"
	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type CompanyBankHandler struct {
	db *sql.DB
}

func NewCompanyBankHandler(db *sql.DB) *CompanyBankHandler {
	return &CompanyBankHandler{db: db}
}
func (h *CompanyBankHandler) List(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("companyId"))
	userID := c.GetInt("user_id")

	owned, err := companyBelongsToUser(h.db, int64(id), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	banks, err := services.GetCompanyBanks(h.db, id, c)
	if err != nil {
		log.Println("failed to fetch company banks:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bank accounts"})
		return
	}

	c.JSON(http.StatusOK, banks)
}
func (h *CompanyBankHandler) Create(c *gin.Context) {
	// Bank details are what customers pay into, so changing them is worth confirming
	// with the password: a stolen session could otherwise quietly point every future
	// invoice at someone else's account. Enforced only when the caller sends one until
	// both apps ask for it — see confirmAccountPassword.
	//
	// Before binding, not after: binding reads the body to the end, so a check that ran
	// afterwards saw no password at all and waved everything through.
	if err := confirmAccountPassword(c, h.db, c.GetInt("user_id")); err != nil {
		return
	}

	var bank models.CompanyBank

	if err := c.ShouldBindJSON(&bank); err != nil {
		c.JSON(400, gin.H{"error": "Invalid input"})
		return
	}

	userID := c.GetInt("user_id")
	owned, err := companyBelongsToUser(h.db, int64(bank.CompanyID), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify company"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	if err := services.CreateCompanyBank(h.db, &bank); err != nil {
		log.Println("failed to create company bank:", err)
		c.JSON(500, gin.H{"error": "Failed to save bank account"})
		return
	}

	c.JSON(200, bank)
}
func (h *CompanyBankHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("bankId"))
	userID := c.GetInt("user_id")

	// Bank details are what customers pay into, so changing them is worth confirming
	// with the password: a stolen session could otherwise quietly point every future
	// invoice at someone else's account. Enforced only when the caller sends one until
	// both apps ask for it — see confirmAccountPassword.
	//
	// Before binding, not after: binding reads the body to the end, so a check that ran
	// afterwards saw no password at all and waved everything through.
	if err := confirmAccountPassword(c, h.db, userID); err != nil {
		return
	}

	owned, err := bankBelongsToUser(h.db, id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify bank account"})
		return
	}
	if !owned {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	var bank models.CompanyBank
	if err := c.ShouldBindJSON(&bank); err != nil {
		c.JSON(400, gin.H{"error": "Invalid input"})
		return
	}

	bank.ID = id

	if err := services.UpdateCompanyBank(h.db, &bank); err != nil {
		log.Println("failed to update company bank:", err)
		c.JSON(500, gin.H{"error": "Failed to update bank account"})
		return
	}

	c.JSON(200, bank)
}
