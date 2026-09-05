package handlers

import (
	"net/http"

	"invo-server/internal/services"

	"github.com/gin-gonic/gin"
)

type DeviceTokenHandler struct {
	pushService *services.PushService
}

func NewDeviceTokenHandler(pushService *services.PushService) *DeviceTokenHandler {
	return &DeviceTokenHandler{pushService: pushService}
}

// POST /api/v1/device-tokens
func (h *DeviceTokenHandler) Register(c *gin.Context) {
	userID := c.GetInt("user_id")

	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	if err := h.pushService.RegisterToken(userID, req.Token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Device registered"})
}

// DELETE /api/v1/device-tokens
func (h *DeviceTokenHandler) Unregister(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	if err := h.pushService.UnregisterToken(req.Token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unregister device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Device unregistered"})
}

// POST /api/v1/push/test
func (h *DeviceTokenHandler) SendTest(c *gin.Context) {
	userID := c.GetInt("user_id")
	h.pushService.SendToUser(userID, "Test notification", "If you see this, push notifications are working 🎉")
	c.JSON(http.StatusOK, gin.H{"message": "Test push requested — check the server log for the result"})
}
