package handler

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/service"
)

type OrderHandler struct {
	svc *service.OrderService
}

func NewOrderHandler(svc *service.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

type OrderItem struct {
	TicketID string `json:"ticket_id" binding:"required,uuid"`
	Quantity int    `json:"quantity" binding:"required,min=1,max=100"`
}

type createOrderRequest struct {
	Items    []OrderItem `json:"items" binding:"required,min=1,max=10,dive"`
	Name     string      `json:"name" binding:"required,min=1,max=255"`
	Email    string      `json:"email" binding:"required,email"`
	Whatsapp string      `json:"whatsapp" binding:"required"`
}

func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !isValidWhatsapp(req.Whatsapp) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "whatsapp must start with 08 and be 10-13 digits"})
		return
	}

	items := make([]service.CreateOrderItemParams, len(req.Items))
	for i, item := range req.Items {
		items[i] = service.CreateOrderItemParams{
			TicketID: item.TicketID,
			Quantity: item.Quantity,
		}
	}

	result, err := h.svc.CreateOrder(c.Request.Context(), service.CreateOrderParams{
		Items:    items,
		Name:     req.Name,
		Email:    req.Email,
		Whatsapp: req.Whatsapp,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, result)
}

func isValidWhatsapp(phone string) bool {
	re := regexp.MustCompile(`^08\d{8,11}$`)
	return re.MatchString(phone)
}
