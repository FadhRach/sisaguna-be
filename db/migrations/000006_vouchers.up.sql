CREATE TABLE vouchers (
    code            text PRIMARY KEY CHECK (code ~ '^[A-Z0-9]{4,20}$'),
    title           text NOT NULL,
    description     text NOT NULL DEFAULT '',
    percent_off     smallint CHECK (percent_off BETWEEN 1 AND 100),
    flat_off        integer CHECK (flat_off > 0),
    max_discount    integer CHECK (max_discount > 0),
    min_spend       integer NOT NULL DEFAULT 0,
    merchant_id     uuid REFERENCES merchants(id) ON DELETE CASCADE,  -- NULL = semua toko
    applies_to_tier listing_tier,                                     -- NULL = semua tier
    starts_at       timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    quota           integer CHECK (quota > 0),                        -- NULL = tanpa batas
    used_count      integer NOT NULL DEFAULT 0,
    image_media_id  uuid REFERENCES media(id) ON DELETE SET NULL,
    terms           text[] NOT NULL DEFAULT '{}',
    accent_hex      text NOT NULL DEFAULT '#55B931',
    is_active       boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CHECK ((percent_off IS NULL) <> (flat_off IS NULL)),
    CHECK (expires_at > starts_at)
);

CREATE TABLE user_vouchers (
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    voucher_code text NOT NULL REFERENCES vouchers(code) ON DELETE CASCADE,
    claimed_at   timestamptz NOT NULL DEFAULT now(),
    used_at      timestamptz,
    order_id     uuid,  -- FK ditambah setelah tabel orders ada
    PRIMARY KEY (user_id, voucher_code)
);
