ALTER TABLE orders ALTER COLUMN ticket_id DROP NOT NULL;

CREATE TABLE order_items (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  ticket_id    UUID NOT NULL REFERENCES tickets(id),
  quantity     INTEGER NOT NULL,
  unit_price   BIGINT NOT NULL,
  subtotal     BIGINT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
