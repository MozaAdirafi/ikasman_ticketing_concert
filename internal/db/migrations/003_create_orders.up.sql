CREATE TABLE orders (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID NOT NULL REFERENCES users(id),
  ticket_id    UUID NOT NULL REFERENCES tickets(id),
  quantity     INTEGER NOT NULL DEFAULT 1,
  total_amount BIGINT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'pending',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
