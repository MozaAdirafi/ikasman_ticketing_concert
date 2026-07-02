-- name: ListTickets :many
SELECT id, name, description, price, stock, created_at FROM tickets ORDER BY created_at ASC;

-- name: GetTicketByID :one
SELECT id, name, description, price, stock, created_at FROM tickets WHERE id = $1 FOR UPDATE;

-- name: DecrementTicketStock :exec
UPDATE tickets SET stock = stock - $2 WHERE id = $1;
