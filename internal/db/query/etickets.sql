-- name: CreateEticket :one
INSERT INTO e_tickets (order_id, user_id, ticket_id, qr_code)
VALUES ($1, $2, $3, $4)
RETURNING id, order_id, user_id, ticket_id, qr_code, is_used, used_at, created_at;

-- name: GetEticketByQRCode :one
SELECT id, order_id, user_id, ticket_id, qr_code, is_used, used_at, created_at FROM e_tickets WHERE qr_code = $1;

-- name: MarkEticketUsed :exec
UPDATE e_tickets SET is_used = TRUE, used_at = $2 WHERE qr_code = $1;

-- name: GetEticketDetails :one
SELECT u.name, t.name
FROM e_tickets e
JOIN users u ON u.id = e.user_id
JOIN tickets t ON t.id = e.ticket_id
WHERE e.qr_code = $1;
