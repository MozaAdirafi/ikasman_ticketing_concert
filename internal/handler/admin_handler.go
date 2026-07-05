package handler

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/MozaAdirafi/ikasman_ticketing_concert/internal/db/sqlc"
	"github.com/MozaAdirafi/ikasman_ticketing_concert/internal/service"
)

type AdminHandler struct {
	q          *db.Queries
	pool       *pgxpool.Pool
	eticketSvc *service.EticketService
}

func NewAdminHandler(q *db.Queries, pool *pgxpool.Pool, eticketSvc *service.EticketService) *AdminHandler {
	return &AdminHandler{q: q, pool: pool, eticketSvc: eticketSvc}
}

type dashboardTicketStats struct {
	TotalTickets int64
	CheckedIn    int64
	Remaining    int64
}

type AttendeeRow struct {
	ID         string     `json:"id"`
	QRCode     string     `json:"qr_code"`
	IsUsed     bool       `json:"is_used"`
	UsedAt     *time.Time `json:"used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	Name       string     `json:"name"`
	Email      string     `json:"email"`
	Whatsapp   string     `json:"whatsapp"`
	OrderID    string     `json:"order_id"`
	TicketType string     `json:"ticket_type"`
}

func (h *AdminHandler) GetDashboard(c *gin.Context) {
	ctx := c.Request.Context()

	rows, err := h.pool.Query(ctx, `
SELECT
	t.name as ticket_name,
	COUNT(et.id) as total_tickets,
	COUNT(CASE WHEN et.is_used = true THEN 1 END) as checked_in,
	COUNT(CASE WHEN et.is_used = false THEN 1 END) as remaining
FROM e_tickets et
JOIN tickets t ON t.id = et.ticket_id
GROUP BY t.name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	totalSold := map[string]int64{}
	totalCheckedIn := map[string]int64{}
	totalRemaining := map[string]int64{}

	for rows.Next() {
		var ticketName string
		var stats dashboardTicketStats
		if err := rows.Scan(&ticketName, &stats.TotalTickets, &stats.CheckedIn, &stats.Remaining); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		key := strings.ToLower(ticketName)
		totalSold[key] = stats.TotalTickets
		totalCheckedIn[key] = stats.CheckedIn
		totalRemaining[key] = stats.Remaining
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var totalRevenue int64
	if err := h.pool.QueryRow(ctx, `SELECT COALESCE(SUM(total_amount), 0) FROM orders WHERE status = 'paid'`).Scan(&totalRevenue); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var lastUpdated time.Time
	if err := h.pool.QueryRow(ctx, `
SELECT COALESCE(MAX(GREATEST(et.created_at, COALESCE(et.used_at, et.created_at))), NOW())
FROM e_tickets et`).Scan(&lastUpdated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_sold":       totalSold,
		"total_checked_in": totalCheckedIn,
		"total_remaining":  totalRemaining,
		"total_revenue":    totalRevenue,
		"last_updated":     lastUpdated.UTC().Format(time.RFC3339),
	})
}

func (h *AdminHandler) GetAttendees(c *gin.Context) {
	attendees, err := h.queryAttendees(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"attendees": attendees,
		"total":     len(attendees),
	})
}

func (h *AdminHandler) ExportAttendees(c *gin.Context) {
	attendees, err := h.queryAttendees(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", `attachment; filename="attendees-kirribilly-2026.csv"`)

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	headers := []string{"No", "Nama", "Email", "WhatsApp", "Jenis Tiket", "Status", "Waktu Check-in", "No. Pesanan"}
	if err := writer.Write(headers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for i, attendee := range attendees {
		status := "Not Checked In"
		usedAt := ""
		if attendee.IsUsed {
			status = "Checked In"
			if attendee.UsedAt != nil {
				usedAt = attendee.UsedAt.UTC().Format(time.RFC3339)
			}
		}

		record := []string{
			strconv.Itoa(i + 1),
			attendee.Name,
			attendee.Email,
			attendee.Whatsapp,
			attendee.TicketType,
			status,
			usedAt,
			attendee.OrderID,
		}
		if err := writer.Write(record); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
}

func (h *AdminHandler) ConfirmPayment(c *gin.Context) {
	orderID := c.Param("order_id")
	ctx := c.Request.Context()

	payment, err := h.q.GetPaymentByOrderID(ctx, orderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
		return
	}

	if err := h.q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: orderID, Status: "paid"}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update order status: %v", err)})
		return
	}

	if err := h.q.UpdatePaymentStatusPaid(ctx, payment.ID.String()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update payment status: %v", err)})
		return
	}

	if err := h.eticketSvc.GenerateAndSend(ctx, orderID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to generate and send e-ticket: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"order_id": orderID,
		"message":  "payment confirmed and e-ticket sent",
	})
}

func (h *AdminHandler) queryAttendees(c *gin.Context) ([]AttendeeRow, error) {
	ctx := c.Request.Context()
	status := strings.TrimSpace(c.DefaultQuery("status", "all"))
	search := strings.TrimSpace(c.Query("search"))

	baseQuery := `
SELECT
	et.id,
	et.qr_code,
	et.is_used,
	et.used_at,
	et.created_at,
	u.name,
	u.email,
	u.whatsapp,
	o.id as order_id,
	t.name as ticket_type
FROM e_tickets et
LEFT JOIN orders o ON o.id = et.order_id
LEFT JOIN users u ON u.id = o.user_id
LEFT JOIN tickets t ON t.id = o.ticket_id`

	whereClauses := []string{}
	args := []interface{}{}

	if status == "checked_in" {
		whereClauses = append(whereClauses, "et.is_used = true")
	} else if status == "not_checked_in" {
		whereClauses = append(whereClauses, "et.is_used = false")
	}

	if search != "" {
		args = append(args, "%"+search+"%")
		whereClauses = append(whereClauses, fmt.Sprintf("u.name ILIKE $%d", len(args)))
	}

	if len(whereClauses) > 0 {
		baseQuery += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}

	baseQuery += "\nORDER BY et.created_at DESC"

	rows, err := h.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attendees := []AttendeeRow{}
	for rows.Next() {
		var item AttendeeRow
		if err := rows.Scan(
			&item.ID,
			&item.QRCode,
			&item.IsUsed,
			&item.UsedAt,
			&item.CreatedAt,
			&item.Name,
			&item.Email,
			&item.Whatsapp,
			&item.OrderID,
			&item.TicketType,
		); err != nil {
			return nil, err
		}
		attendees = append(attendees, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(attendees) > 0 {
		log.Printf("[DEBUG] First attendee: name=%s, whatsapp=%s, order=%s", attendees[0].Name, attendees[0].Whatsapp, attendees[0].OrderID)
	}

	log.Printf("[INFO] Admin attendees query returned %d row(s) (status=%s, search=%q)", len(attendees), status, search)

	return attendees, nil
}
