package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	db "github.com/MozaAdirafi/ikasman_ticketing_concert/internal/db/sqlc"
)

type PaymentService struct {
	q *db.Queries
}

func NewPaymentService(q *db.Queries) *PaymentService {
	return &PaymentService{q: q}
}

type PaymentStatusResult struct {
	OrderID       string `json:"order_id"`
	PaymentStatus string `json:"payment_status"`
	OrderStatus   string `json:"order_status"`
	PaymentURL    string `json:"payment_url,omitempty"`
}

func (s *PaymentService) GetPaymentStatus(ctx context.Context, orderID string) (*PaymentStatusResult, error) {
	payment, err := s.q.GetPaymentByOrderID(ctx, orderID)
	if err != nil {
		return nil, errors.New("payment not found")
	}

	order, err := s.q.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, errors.New("order not found")
	}

	result := &PaymentStatusResult{
		OrderID:       orderID,
		PaymentStatus: payment.Status,
		OrderStatus:   order.Status,
	}
	if payment.PaymentUrl != "" {
		result.PaymentURL = payment.PaymentUrl
	}

	return result, nil
}

type MidtransWebhookPayload struct {
	OrderID           string `json:"order_id"`
	TransactionID     string `json:"transaction_id"`
	TransactionTime   string `json:"transaction_time"`
	TransactionStatus string `json:"transaction_status"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
}

func (s *PaymentService) HandleWebhook(ctx context.Context, r *http.Request, eticketSvc *EticketService) error {
	var payload MidtransWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return errors.New("invalid webhook payload")
	}

	// Convert gross_amount string to int64
	var grossAmount int64
	var grossAmountFloat float64
	if err := json.Unmarshal([]byte(payload.GrossAmount), &grossAmountFloat); err != nil {
		log.Printf("[WARN] Failed to parse gross_amount: %v", err)
		grossAmount = 0
	} else {
		grossAmount = int64(grossAmountFloat)
	}
	log.Printf("[DEBUG] Parsed gross_amount: %s -> %.2f -> %d", payload.GrossAmount, grossAmountFloat, grossAmount)

	// Verify Midtrans signature using status_code (NOT transaction_status)
	if !VerifyMidtransSignature(payload.OrderID, payload.StatusCode, payload.GrossAmount, payload.SignatureKey) {
		return errors.New("invalid signature")
	}

	payment, err := s.q.GetPaymentByOrderID(ctx, payload.OrderID)
	if err != nil {
		return errors.New("payment not found for order")
	}

	// Payment successful: "settlement" or "capture"
	if payload.TransactionStatus == "settlement" || payload.TransactionStatus == "capture" {
		if err := s.q.UpdatePaymentStatusPaid(ctx, payment.ID.String()); err != nil {
			return err
		}
		if err := s.q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
			ID:     payload.OrderID,
			Status: "paid",
		}); err != nil {
			return err
		}

		if err := eticketSvc.GenerateAndSend(ctx, payload.OrderID); err != nil {
			log.Printf("[ERROR] Failed to generate/send etickets for order %s: %v", payload.OrderID, err)
		}
		log.Printf("[INFO] Payment confirmed for order: %s", payload.OrderID)
	} else if payload.TransactionStatus == "deny" || payload.TransactionStatus == "cancel" || payload.TransactionStatus == "expire" {
		if err := s.q.UpdatePaymentStatusFailed(ctx, payment.ID.String()); err != nil {
			return err
		}
		if err := s.q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
			ID:     payload.OrderID,
			Status: "failed",
		}); err != nil {
			return err
		}
		log.Printf("[INFO] Payment failed for order: %s, status: %s", payload.OrderID, payload.TransactionStatus)
	} else {
		log.Printf("[INFO] Payment pending for order: %s, status: %s", payload.OrderID, payload.TransactionStatus)
	}

	return nil
}
