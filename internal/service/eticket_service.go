package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	resend "github.com/resend/resend-go/v2"

	db "github.com/MozaAdirafi/ikasman_ticketing_concert/internal/db/sqlc"
)

type EticketService struct {
	q *db.Queries
}

func NewEticketService(q *db.Queries) *EticketService {
	return &EticketService{q: q}
}

type CheckinResult struct {
	Valid        bool   `json:"valid"`
	Message      string `json:"message,omitempty"`
	TicketHolder string `json:"ticket_holder,omitempty"`
	TicketType   string `json:"ticket_type,omitempty"`
}

func (s *EticketService) GenerateAndSend(ctx context.Context, orderID string) error {
	log.Printf("[INFO]   Step 4a: Fetching order details for order_id: %s", orderID)
	order, err := s.q.GetOrderWithDetails(ctx, orderID)
	if err != nil {
		log.Printf("[ERROR]   Step 4a FAILED: Order not found: %v", err)
		return fmt.Errorf("order not found: %w", err)
	}
	log.Printf("[INFO]   Step 4b: Order found - user_id: %s, ticket_id: %s, quantity: %d", order.UserID, order.TicketID, 1)
	log.Printf("[INFO]       User: %s (%s)", order.UserName, order.UserEmail)

	log.Printf("[INFO]   Step 4c: Generating QR code")
	qrValue := uuid.New().String()

	if _, err := s.q.CreateEticket(ctx, db.CreateEticketParams{
		OrderID:  order.ID,
		UserID:   order.UserID,
		TicketID: order.TicketID,
		QrCode:   qrValue,
	}); err != nil {
		log.Printf("[ERROR]   Step 4c FAILED: Could not create eticket record: %v", err)
		return fmt.Errorf("failed to create eticket: %w", err)
	}

	qrPNG, err := qrcode.Encode(qrValue, qrcode.Medium, 256)
	if err != nil {
		log.Printf("[ERROR]   Step 4c FAILED: Could not generate QR code: %v", err)
		return fmt.Errorf("failed to generate QR code: %w", err)
	}
	log.Printf("[INFO]   Step 4d: QR code generated successfully (qr_value: %s)", qrValue)

	log.Printf("[INFO]   Step 4e: Sending email to %s", order.UserEmail)
	if err := sendEticketEmail(order.UserEmail, order.UserName, order.TicketName, qrPNG); err != nil {
		log.Printf("[ERROR]   Step 4e FAILED: Could not send eticket email: %v", err)
		return fmt.Errorf("failed to send eticket email: %w", err)
	}
	log.Printf("[INFO]   Step 4f: Email sent successfully to %s", order.UserEmail)

	return nil
}

func (s *EticketService) Checkin(ctx context.Context, qrCode string) (*CheckinResult, error) {
	eticket, err := s.q.GetEticketByQRCode(ctx, qrCode)
	if err != nil {
		return &CheckinResult{Valid: false, Message: "Ticket not found"}, nil
	}

	if eticket.IsUsed {
		return &CheckinResult{Valid: false, Message: "Ticket already used"}, nil
	}

	if err := s.q.MarkEticketUsed(ctx, db.MarkEticketUsedParams{
		QrCode: qrCode,
		UsedAt: time.Now(),
	}); err != nil {
		return nil, fmt.Errorf("failed to mark ticket used: %w", err)
	}

	details, err := s.q.GetEticketDetails(ctx, qrCode)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticket details: %w", err)
	}

	return &CheckinResult{
		Valid:        true,
		TicketHolder: details.UserName,
		TicketType:   details.TicketName,
	}, nil
}

func sendEticketEmail(toEmail, name, ticketType string, qrPNG []byte) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return errors.New("RESEND_API_KEY not set")
	}

	fromEmail := os.Getenv("FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "tickets@yourdomain.com"
	}

	client := resend.NewClient(apiKey)

	htmlBody := fmt.Sprintf(`
		<h2>Your E-Ticket</h2>
		<p>Hi %s,</p>
		<p>Thank you for your purchase! Your ticket type: <strong>%s</strong></p>
		<p>Please present the attached QR code at the entrance.</p>
	`, name, ticketType)

	params := &resend.SendEmailRequest{
		From:    fromEmail,
		To:      []string{toEmail},
		Subject: "Your Concert E-Ticket",
		Html:    htmlBody,
		Attachments: []*resend.Attachment{
			{
				Filename: "eticket-qr.png",
				Content:  qrPNG,
			},
		},
	}

	_, err := client.Emails.Send(params)
	return err
}
