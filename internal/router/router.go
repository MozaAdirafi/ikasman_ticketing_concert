package router

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/MozaAdirafi/ikasman_ticketing_concert/internal/db/sqlc"
	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/handler"
	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/middleware"
	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/service"
)

func SetupRouter(pool *pgxpool.Pool) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.CORS())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	queries := db.New(pool)

	ticketSvc := service.NewTicketService(queries)
	orderSvc := service.NewOrderService(queries, pool)
	paymentSvc := service.NewPaymentService(queries)
	eticketSvc := service.NewEticketService(queries)

	ticketHandler := handler.NewTicketHandler(ticketSvc)
	orderHandler := handler.NewOrderHandler(orderSvc)
	paymentHandler := handler.NewPaymentHandler(paymentSvc, eticketSvc)
	checkinHandler := handler.NewCheckinHandler(eticketSvc)

	api := r.Group("/api/v1")
	{
		api.GET("/tickets", ticketHandler.ListTickets)

		api.POST("/orders", orderHandler.CreateOrder)

		api.GET("/payments/status/:order_id", paymentHandler.GetPaymentStatus)
		api.POST("/payments/webhook", paymentHandler.HandleWebhook)

		api.POST("/checkin", middleware.AdminAuth(), checkinHandler.Checkin)
	}

	return r
}
