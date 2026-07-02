package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/service"
)

type CheckinHandler struct {
	svc *service.EticketService
}

func NewCheckinHandler(svc *service.EticketService) *CheckinHandler {
	return &CheckinHandler{svc: svc}
}

type checkinRequest struct {
	QRCode string `json:"qr_code" binding:"required"`
}

func (h *CheckinHandler) Checkin(c *gin.Context) {
	var req checkinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.svc.Checkin(c.Request.Context(), req.QRCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
