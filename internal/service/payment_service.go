package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

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
		log.Printf("[ERROR] Failed to decode webhook payload: %v", err)
		return errors.New("invalid webhook payload")
	}

	log.Printf("[INFO] ========== WEBHOOK RECEIVED ==========")
	log.Printf("[INFO] OrderID: %s", payload.OrderID)
	log.Printf("[INFO] TransactionStatus: %s", payload.TransactionStatus)
	log.Printf("[INFO] StatusCode: %s", payload.StatusCode)
	log.Printf("[INFO] GrossAmount: %s", payload.GrossAmount)

	// Check if this is a test notification from Midtrans
	if strings.HasPrefix(payload.OrderID, "payment_notif_test_") {
		log.Printf("[INFO] Test webhook received - ignoring (order_id: %s)", payload.OrderID)
		log.Printf("[INFO] Midtrans endpoint connectivity confirmed ✓")
		return nil
	}

	// Convert gross_amount string to int64 (for logging/reference only)
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
	log.Printf("[INFO] Verifying Midtrans signature...")
	if !VerifyMidtransSignature(payload.OrderID, payload.StatusCode, payload.GrossAmount, payload.SignatureKey) {
		log.Printf("[ERROR] Signature verification FAILED")
		return errors.New("invalid signature")
	}
	log.Printf("[INFO] ✓ Signature verified")

	// Payment successful: "settlement" or "capture"
	if payload.TransactionStatus == "settlement" || payload.TransactionStatus == "capture" {
		log.Printf("[INFO] Processing SUCCESSFUL payment...")

		// Step 1: Look up payment
		log.Printf("[INFO] Step 1: Looking up payment by order_id: %s", payload.OrderID)
		payment, err := s.q.GetPaymentByOrderID(ctx, payload.OrderID)
		if err != nil {
			log.Printf("[ERROR] Step 1 FAILED: Payment NOT found for order %s: %v", payload.OrderID, err)
			return errors.New("payment not found for order")
		}
		log.Printf("[INFO] Step 1: ✓ Payment found (payment_id: %s)", payment.ID)

		// Step 2: Update payment status to paid
		log.Printf("[INFO] Step 2: Updating payment status to 'paid'...")
		if err := s.q.UpdatePaymentStatusPaid(ctx, payment.ID.String()); err != nil {
			log.Printf("[ERROR] Step 2 FAILED: Could not update payment status: %v", err)
			return err
		}
		log.Printf("[INFO] Step 2: ✓ Payment status updated to 'paid'")

		// Step 3: Update order status to paid
		log.Printf("[INFO] Step 3: Updating order status to 'paid'...")
		if err := s.q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
			ID:     payload.OrderID,
			Status: "paid",
		}); err != nil {
			log.Printf("[ERROR] Step 3 FAILED: Could not update order status: %v", err)
			return err
		}
		log.Printf("[INFO] Step 3: ✓ Order status updated to 'paid'")

		// Step 4: Generate and send e-tickets
		log.Printf("[INFO] Step 4: Generating and sending e-tickets...")
		if err := eticketSvc.GenerateAndSend(ctx, payload.OrderID); err != nil {
			log.Printf("[ERROR] Step 4 FAILED: Failed to generate/send e-tickets: %v", err)
			return err
		}
		log.Printf("[INFO] Step 4: ✓ E-tickets generated and sent successfully")

		log.Printf("[INFO] ========== PAYMENT PROCESSING COMPLETE ==========")
		log.Printf("[INFO] Payment confirmed for order: %s", payload.OrderID)

	} else if payload.TransactionStatus == "deny" || payload.TransactionStatus == "cancel" || payload.TransactionStatus == "expire" {
		log.Printf("[INFO] Processing FAILED payment (status: %s)...", payload.TransactionStatus)

		log.Printf("[INFO] Step 1: Looking up payment by order_id: %s", payload.OrderID)
		payment, err := s.q.GetPaymentByOrderID(ctx, payload.OrderID)
		if err != nil {
			log.Printf("[ERROR] Step 1 FAILED: Payment NOT found for order %s: %v", payload.OrderID, err)
			return errors.New("payment not found for order")
		}
		log.Printf("[INFO] Step 1: ✓ Payment found (payment_id: %s)", payment.ID)

		log.Printf("[INFO] Step 2: Updating payment status to 'failed'...")
		if err := s.q.UpdatePaymentStatusFailed(ctx, payment.ID.String()); err != nil {
			log.Printf("[ERROR] Step 2 FAILED: Could not update payment status: %v", err)
			return err
		}
		log.Printf("[INFO] Step 2: ✓ Payment status updated to 'failed'")

		log.Printf("[INFO] Step 3: Updating order status to 'failed'...")
		if err := s.q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
			ID:     payload.OrderID,
			Status: "failed",
		}); err != nil {
			log.Printf("[ERROR] Step 3 FAILED: Could not update order status: %v", err)
			return err
		}
		log.Printf("[INFO] Step 3: ✓ Order status updated to 'failed'")

		log.Printf("[INFO] ========== PAYMENT PROCESSING COMPLETE ==========")
		log.Printf("[INFO] Payment failed for order: %s, transaction_status: %s", payload.OrderID, payload.TransactionStatus)

	} else {
		log.Printf("[INFO] Payment in PENDING state (status: %s)", payload.TransactionStatus)
		log.Printf("[INFO] No action taken. Waiting for final status...")
	}

	return nil
}
