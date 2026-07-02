-- name: CreatePayment :one
INSERT INTO payments (order_id, doku_invoice_no, payment_url)
VALUES ($1, $2, $3)
RETURNING id, order_id, doku_invoice_no, payment_url, status, paid_at, created_at;

-- name: GetPaymentByOrderID :one
SELECT id, order_id, doku_invoice_no, payment_url, status, paid_at, created_at FROM payments WHERE order_id = $1;

-- name: GetPaymentByInvoiceNo :one
SELECT id, order_id, doku_invoice_no, payment_url, status, paid_at, created_at FROM payments WHERE doku_invoice_no = $1;

-- name: UpdatePaymentStatusPaid :exec
UPDATE payments SET status = 'paid', paid_at = NOW() WHERE id = $1;

-- name: UpdatePaymentStatusFailed :exec
UPDATE payments SET status = 'failed' WHERE id = $1;
