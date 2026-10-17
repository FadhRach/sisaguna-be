CREATE TABLE conversations (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    merchant_id           uuid NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    last_message_id       bigint,
    last_message_at       timestamptz,
    buyer_last_read_id    bigint NOT NULL DEFAULT 0,
    merchant_last_read_id bigint NOT NULL DEFAULT 0,
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (buyer_id, merchant_id)
);
CREATE INDEX conversations_buyer_idx    ON conversations (buyer_id, last_message_at DESC);
CREATE INDEX conversations_merchant_idx ON conversations (merchant_id, last_message_at DESC);

CREATE TABLE messages (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- urut, dipakai sebagai cursor
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_side     chat_side NOT NULL,
    sender_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body            text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    listing_id      uuid REFERENCES listings(id) ON DELETE SET NULL,
    client_id       uuid NOT NULL,           -- dibuat HP, mencegah pesan dobel saat kirim ulang
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, client_id)
);
CREATE INDEX messages_conversation_idx ON messages (conversation_id, id);
