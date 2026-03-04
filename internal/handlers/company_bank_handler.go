package handlers

import (
	"database/sql"
	"fmt"
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

	banks, err := services.GetCompanyBanks(h.db, id, c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, banks)
}
func (h *CompanyBankHandler) Create(c *gin.Context) {
	var bank models.CompanyBank

	if err := c.ShouldBindJSON(&bank); err != nil {
		fmt.Println("Error binding JSON:", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if err := services.CreateCompanyBank(h.db, &bank); err != nil {
		fmt.Println("Error creating company bank:", err)
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, bank)
}
func (h *CompanyBankHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("bankId"))

	var bank models.CompanyBank
	if err := c.ShouldBindJSON(&bank); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	bank.ID = id

	if err := services.UpdateCompanyBank(h.db, &bank); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, bank)
}
