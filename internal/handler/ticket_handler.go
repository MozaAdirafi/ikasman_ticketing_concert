package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/service"
)

type TicketHandler struct {
	svc *service.TicketService
}

func NewTicketHandler(svc *service.TicketService) *TicketHandler {
	return &TicketHandler{svc: svc}
}

func (h *TicketHandler) ListTickets(c *gin.Context) {
	tickets, err := h.svc.ListTickets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tickets)
}
