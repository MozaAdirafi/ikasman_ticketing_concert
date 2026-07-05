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
	adminHandler := handler.NewAdminHandler(queries, pool, eticketSvc)

	api := r.Group("/api/v1")
	{
		api.GET("/tickets", ticketHandler.ListTickets)

		api.POST("/orders", orderHandler.CreateOrder)

		api.GET("/payments/status/:order_id", paymentHandler.GetPaymentStatus)
		api.POST("/payments/webhook", paymentHandler.HandleWebhook)

		api.OPTIONS("/checkin", func(c *gin.Context) {
			c.Status(204)
		})

		adminPreflight := api.Group("/admin")
		adminPreflight.OPTIONS("/*path", func(c *gin.Context) {
			c.Status(204)
		})

		api.POST("/checkin", middleware.AdminAuth(), checkinHandler.Checkin)

		admin := api.Group("/admin", middleware.AdminAuth())
		{
			admin.GET("/dashboard", adminHandler.GetDashboard)
			admin.GET("/attendees", adminHandler.GetAttendees)
			admin.GET("/attendees/export", adminHandler.ExportAttendees)
			admin.POST("/payment/confirm/:order_id", adminHandler.ConfirmPayment)
		}
	}

	return r
}
