package service

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/MozaAdirafi/ikasman_ticketing_concert/internal/db/sqlc"
)

type OrderService struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

func NewOrderService(q *db.Queries, pool *pgxpool.Pool) *OrderService {
	return &OrderService{q: q, pool: pool}
}

type CreateOrderItemParams struct {
	TicketID string
	Quantity int
}

type CreateOrderParams struct {
	Items    []CreateOrderItemParams
	Name     string
	Email    string
	Whatsapp string
}

type OrderItemResult struct {
	Ticket struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Price int64  `json:"price"`
	} `json:"ticket"`
	Quantity int `json:"quantity"`
}

type CreateOrderResult struct {
	OrderID     string             `json:"order_id"`
	PaymentURL  string             `json:"payment_url"`
	TotalAmount int64              `json:"total_amount"`
	Items       []OrderItemResult  `json:"items"`
}

func (s *OrderService) CreateOrder(ctx context.Context, params CreateOrderParams) (*CreateOrderResult, error) {
	if len(params.Items) == 0 {
		return nil, errors.New("at least one item is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.q.WithTx(tx)

	var totalAmount int64
	ticketMap := make(map[string]db.Ticket)
	itemResults := make([]OrderItemResult, len(params.Items))

	for i, item := range params.Items {
		ticket, err := s.getTicketWithLock(ctx, tx, item.TicketID)
		if err != nil {
			return nil, err
		}

		if int(ticket.Stock) < item.Quantity {
			return nil, fmt.Errorf("insufficient stock for ticket %s", ticket.Name)
		}

		ticketMap[item.TicketID] = ticket
		itemAmount := ticket.Price * int64(item.Quantity)
		totalAmount += itemAmount

		if err := qtx.DecrementTicketStock(ctx, db.DecrementTicketStockParams{
			ID:    item.TicketID,
			Stock: int32(item.Quantity),
		}); err != nil {
			return nil, fmt.Errorf("failed to decrement stock for ticket %s: %w", ticket.Name, err)
		}

		itemResults[i] = OrderItemResult{
			Quantity: item.Quantity,
		}
		itemResults[i].Ticket.ID = item.TicketID
		itemResults[i].Ticket.Name = ticket.Name
		itemResults[i].Ticket.Price = ticket.Price
	}

	user, err := qtx.UpsertUser(ctx, db.UpsertUserParams{
		Name:     params.Name,
		Email:    params.Email,
		Whatsapp: params.Whatsapp,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upsert user: %w", err)
	}

	order, err := qtx.CreateOrder(ctx, db.CreateOrderParams{
		UserID:      user.ID,
		TicketID:    nil,
		Quantity:    1,
		TotalAmount: totalAmount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	for i, item := range params.Items {
		ticket := ticketMap[item.TicketID]
		_, err := qtx.CreateOrderItem(ctx, db.CreateOrderItemParams{
			OrderID:   order.ID,
			TicketID:  ticket.ID,
			Quantity:  int32(item.Quantity),
			UnitPrice: ticket.Price,
			Subtotal:  ticket.Price * int64(item.Quantity),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create order item %d: %w", i, err)
		}
	}

	paymentURL, err := createMidtransTransaction(order.ID.String(), totalAmount, params.Name, params.Email, params.Whatsapp, itemResults)
	if err != nil {
		return nil, fmt.Errorf("failed to create Midtrans transaction: %w", err)
	}

	if _, err := qtx.CreatePayment(ctx, db.CreatePaymentParams{
		OrderID:       order.ID,
		DokuInvoiceNo: order.ID.String(),
		PaymentUrl:    paymentURL,
	}); err != nil {
		return nil, fmt.Errorf("failed to create payment record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("[INFO] Order created: %s, total_amount: %d, items: %d", order.ID, totalAmount, len(params.Items))

	return &CreateOrderResult{
		OrderID:     order.ID.String(),
		PaymentURL:  paymentURL,
		TotalAmount: totalAmount,
		Items:       itemResults,
	}, nil
}

func (s *OrderService) getTicketWithLock(ctx context.Context, tx pgx.Tx, ticketID string) (db.Ticket, error) {
	var ticket db.Ticket
	query := `SELECT id, name, description, price, stock, created_at FROM tickets WHERE id = $1 FOR UPDATE`
	err := tx.QueryRow(ctx, query, ticketID).Scan(
		&ticket.ID, &ticket.Name, &ticket.Description, &ticket.Price, &ticket.Stock, &ticket.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ticket, errors.New("ticket not found")
		}
		return ticket, fmt.Errorf("failed to fetch ticket: %w", err)
	}
	return ticket, nil
}

func createMidtransTransaction(orderID string, grossAmount int64, customerName, customerEmail, customerPhone string, items []OrderItemResult) (string, error) {
	serverKey := os.Getenv("MIDTRANS_SERVER_KEY")
	if serverKey == "" {
		return "", errors.New("MIDTRANS_SERVER_KEY not set")
	}

	itemList := make([]map[string]interface{}, len(items))
	for i, item := range items {
		itemList[i] = map[string]interface{}{
			"id":       item.Ticket.ID,
			"name":     item.Ticket.Name,
			"price":    item.Ticket.Price,
			"quantity": item.Quantity,
			"category": "ticket",
		}
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3001"
	}

	payload := map[string]interface{}{
		"transaction_details": map[string]interface{}{
			"order_id":      orderID,
			"gross_amount":  grossAmount,
		},
		"customer_details": map[string]interface{}{
			"first_name": customerName,
			"email":      customerEmail,
			"phone":      customerPhone,
		},
		"item_details": itemList,
		"redirect_url": frontendURL + "/checkout?order_id=" + orderID,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://app.sandbox.midtrans.com/snap/v1/transactions", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth header
	auth := base64.StdEncoding.EncodeToString([]byte(serverKey + ":"))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Midtrans API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("Midtrans API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	redirectURL, ok := result["redirect_url"].(string)
	if !ok || redirectURL == "" {
		return "", fmt.Errorf("no redirect_url in Midtrans response: %s", string(body))
	}

	log.Printf("[INFO] Midtrans transaction created: order_id=%s, redirect_url=%s", orderID, redirectURL)

	return redirectURL, nil
}

func VerifyMidtransSignature(orderID, statusCode string, grossAmount int64, signature string) bool {
	serverKey := os.Getenv("MIDTRANS_SERVER_KEY")
	if serverKey == "" {
		log.Printf("[ERROR] MIDTRANS_SERVER_KEY not set")
		return false
	}

	signatureString := orderID + statusCode + fmt.Sprintf("%d", grossAmount) + serverKey
	hash := sha512.Sum512([]byte(signatureString))
	expectedSignature := hex.EncodeToString(hash[:])

	isValid := expectedSignature == signature
	log.Printf("[DEBUG] Signature verification: orderID=%s, statusCode=%s, grossAmount=%d", orderID, statusCode, grossAmount)
	log.Printf("[DEBUG] Signature string: %s", signatureString)
	if !isValid {
		log.Printf("[WARN] Midtrans signature mismatch. Expected: %s, Got: %s", expectedSignature, signature)
	} else {
		log.Printf("[INFO] Midtrans signature verified successfully")
	}
	return isValid
}
