package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/service"
)

type PaymentHandler struct {
	svc       *service.PaymentService
	eticketSvc *service.EticketService
}

func NewPaymentHandler(svc *service.PaymentService, eticketSvc *service.EticketService) *PaymentHandler {
	return &PaymentHandler{svc: svc, eticketSvc: eticketSvc}
}

func (h *PaymentHandler) GetPaymentStatus(c *gin.Context) {
	orderID := c.Param("order_id")
	status, err := h.svc.GetPaymentStatus(c.Request.Context(), orderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *PaymentHandler) HandleWebhook(c *gin.Context) {
	if err := h.svc.HandleWebhook(c.Request.Context(), c.Request, h.eticketSvc); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
