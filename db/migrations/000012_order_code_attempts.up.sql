-- Percobaan kode pickup yang salah, untuk mengunci konfirmasi (5 salah dalam 10 menit).
CREATE TABLE order_code_attempts (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id     uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    attempted_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_code_attempts_idx ON order_code_attempts (order_id, attempted_at);
