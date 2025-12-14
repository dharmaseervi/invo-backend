package handlers

import (
	database "invo-server/internal/db"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	db *database.Database
}

func NewUserHandler(db *database.Database) *UserHandler {
	return &UserHandler{db: db}
}

// GET /profile
func (h *UserHandler) GetUserProfile(c *gin.Context) {
	userID, _ := c.Get("user_id") // from JWT middleware
	email, _ := c.Get("email")

	c.JSON(http.StatusOK, gin.H{
		"user_id": userID,
		"email":   email,
	})
}
