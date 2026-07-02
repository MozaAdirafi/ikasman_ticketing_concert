CREATE TABLE e_tickets (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     UUID NOT NULL REFERENCES orders(id),
  user_id      UUID NOT NULL REFERENCES users(id),
  ticket_id    UUID NOT NULL REFERENCES tickets(id),
  qr_code      TEXT NOT NULL UNIQUE,
  is_used      BOOLEAN NOT NULL DEFAULT FALSE,
  used_at      TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
