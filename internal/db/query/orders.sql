-- name: UpsertUser :one
INSERT INTO users (name, email, whatsapp)
VALUES ($1, $2, $3)
ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, whatsapp = EXCLUDED.whatsapp
RETURNING id, name, email, whatsapp, created_at;

-- name: CreateOrder :one
INSERT INTO orders (user_id, ticket_id, quantity, total_amount)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, ticket_id, quantity, total_amount, status, created_at;

-- name: GetOrderByID :one
SELECT id, user_id, ticket_id, quantity, total_amount, status, created_at FROM orders WHERE id = $1;

-- name: UpdateOrderStatus :exec
UPDATE orders SET status = $2 WHERE id = $1;

-- name: GetOrderWithDetails :one
SELECT o.id, o.user_id, o.ticket_id, u.name, u.email, t.name
FROM orders o
JOIN users u ON u.id = o.user_id
JOIN tickets t ON t.id = o.ticket_id
WHERE o.id = $1;
