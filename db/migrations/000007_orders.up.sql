CREATE TABLE orders (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code               text NOT NULL CHECK (code ~ '^SG-[0-9]{4}$'),
    code_date          date NOT NULL DEFAULT (now() AT TIME ZONE 'Asia/Jakarta')::date,
    buyer_id           uuid NOT NULL REFERENCES users(id),
    merchant_id        uuid NOT NULL REFERENCES merchants(id),
    status             order_status NOT NULL,
    fulfillment        fulfillment NOT NULL DEFAULT 'pickup',
    payment_kind       payment_kind NOT NULL,
    payment_status     payment_status NOT NULL DEFAULT 'unpaid',
    note               text NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    subtotal           integer NOT NULL CHECK (subtotal >= 0),
    original_total     integer NOT NULL CHECK (original_total >= subtotal),
    voucher_code       text REFERENCES vouchers(code),
    voucher_discount   integer NOT NULL DEFAULT 0 CHECK (voucher_discount BETWEEN 0 AND subtotal),
    service_fee        integer NOT NULL CHECK (service_fee >= 0),
    delivery_fee       integer NOT NULL DEFAULT 0 CHECK (delivery_fee >= 0),
    total              integer NOT NULL,
    commission         integer NOT NULL CHECK (commission >= 0),  -- 10% dari subtotal - voucher_discount
    merchant_payout    integer NOT NULL CHECK (merchant_payout >= 0),
    delivery_courier   courier,
    delivery_speed     delivery_speed,
    delivery_eta_min   smallint,
    delivery_eta_max   smallint,
    delivery_address   jsonb,                   -- salinan alamat saat checkout
    pickup_by          timestamptz,             -- batas datang untuk pickup
    pickup_end         timestamptz NOT NULL,    -- jendela listing paling awal tutup
    idempotency_key    text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    paid_at            timestamptz,
    completed_at       timestamptz,
    cancelled_at       timestamptz,
    cancelled_by       chat_side,
    cancel_reason      text,
    reminded_at        timestamptz,             -- reminder pickup sudah terkirim
    voucher_funded_by  text CHECK (voucher_funded_by IN ('platform', 'merchant')),
    UNIQUE (buyer_id, idempotency_key),
    UNIQUE (merchant_id, code_date, code),
    CHECK (total = subtotal - voucher_discount + service_fee + delivery_fee),
    CHECK ((fulfillment = 'pickup' AND pickup_by IS NOT NULL AND delivery_courier IS NULL)
        OR (fulfillment = 'delivery' AND delivery_courier IS NOT NULL AND delivery_address IS NOT NULL))
);
CREATE INDEX orders_buyer_idx     ON orders (buyer_id, created_at DESC);
CREATE INDEX orders_merchant_idx  ON orders (merchant_id, status, created_at DESC);
CREATE INDEX orders_due_idx       ON orders (pickup_by) WHERE status = 'ready';

CREATE TABLE order_items (
    order_id            uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    listing_id          uuid NOT NULL REFERENCES listings(id),
    title               text NOT NULL,
    tier                listing_tier NOT NULL,
    unit                quantity_unit NOT NULL,
    image_media_id      uuid REFERENCES media(id) ON DELETE SET NULL,
    quantity            integer NOT NULL CHECK (quantity > 0),
    unit_price          integer NOT NULL CHECK (unit_price >= 0),
    unit_original_price integer NOT NULL CHECK (unit_original_price >= unit_price),
    PRIMARY KEY (order_id, listing_id)
);

ALTER TABLE user_vouchers ADD CONSTRAINT user_vouchers_order_fk FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE SET NULL;
