CREATE TABLE reviews (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    uuid NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    merchant_id uuid NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    buyer_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stars       smallint NOT NULL CHECK (stars BETWEEN 1 AND 5),
    tags        text[] NOT NULL DEFAULT '{}',
    comment     text NOT NULL DEFAULT '' CHECK (char_length(comment) <= 500),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reviews_merchant_idx ON reviews (merchant_id, created_at DESC);

CREATE TABLE review_media (
    review_id uuid NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    media_id  uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    position  smallint NOT NULL CHECK (position BETWEEN 0 AND 2),
    PRIMARY KEY (review_id, media_id)
);

CREATE TABLE complaints (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        uuid NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    buyer_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    merchant_id     uuid NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    reason          complaint_reason NOT NULL,
    detail          text NOT NULL CHECK (char_length(detail) BETWEEN 10 AND 1000),
    status          complaint_status NOT NULL DEFAULT 'submitted',
    resolution_note text,
    refund_amount   integer NOT NULL DEFAULT 0 CHECK (refund_amount >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    resolved_at     timestamptz
);
CREATE INDEX complaints_merchant_idx ON complaints (merchant_id, created_at DESC);

CREATE TABLE complaint_media (
    complaint_id uuid NOT NULL REFERENCES complaints(id) ON DELETE CASCADE,
    media_id     uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,  -- visibility = 'private'
    position     smallint NOT NULL CHECK (position BETWEEN 0 AND 2),
    PRIMARY KEY (complaint_id, media_id)
);
