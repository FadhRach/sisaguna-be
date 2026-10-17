-- Penyesuaian dengan app Android: potongan ongkir, label voucher, baseline impact, metode pembayaran.
ALTER TABLE orders ADD COLUMN delivery_discount integer NOT NULL DEFAULT 0 CHECK (delivery_discount >= 0);  -- delivery_fee = ongkir yang dibayar; fee asli = fee + discount

ALTER TABLE vouchers ADD COLUMN applies_to_label text NOT NULL DEFAULT '';  -- teks tampilan, mis. 'Roti, kue & dessert'

-- Impact sebelum pengguna memakai backend ini; ditambahkan ke angka dari order selesai.
ALTER TABLE users
    ADD COLUMN impact_base_portions       integer NOT NULL DEFAULT 0,
    ADD COLUMN impact_base_feed_kg        integer NOT NULL DEFAULT 0,
    ADD COLUMN impact_base_saved_rupiah   integer NOT NULL DEFAULT 0;

CREATE TABLE payment_methods (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       payment_kind NOT NULL,
    detail     text NOT NULL DEFAULT '',   -- mis. '0812 •••• 7890'
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind)
);
CREATE UNIQUE INDEX payment_methods_one_default ON payment_methods (user_id) WHERE is_default;

DROP VIEW user_impact;
CREATE VIEW user_impact AS
WITH done AS (
    SELECT id, buyer_id, voucher_discount FROM orders WHERE status = 'completed'
), per_order AS (
    SELECT buyer_id, count(*) AS completed_orders, sum(voucher_discount) AS voucher_savings
    FROM done GROUP BY buyer_id
), per_item AS (
    SELECT d.buyer_id,
           sum(oi.quantity) FILTER (WHERE oi.tier = 'human')            AS portions,
           sum(oi.quantity) FILTER (WHERE oi.tier <> 'human')           AS feed_compost_kg,
           sum((oi.unit_original_price - oi.unit_price) * oi.quantity) AS item_savings
    FROM done d JOIN order_items oi ON oi.order_id = d.id
    GROUP BY d.buyer_id
)
SELECT u.id AS user_id,
       coalesce(o.completed_orders, 0)                                                              AS completed_orders,
       coalesce(i.portions, 0) + u.impact_base_portions                                             AS portions,
       coalesce(i.feed_compost_kg, 0) + u.impact_base_feed_kg                                       AS feed_compost_kg,
       coalesce(i.item_savings, 0) + coalesce(o.voucher_savings, 0) + u.impact_base_saved_rupiah    AS saved_rupiah
FROM users u
LEFT JOIN per_order o ON o.buyer_id = u.id
LEFT JOIN per_item i ON i.buyer_id = u.id;

-- Listing gratis boleh menyimpan harga normal (app Android menampilkan "Rp20.000 -> Gratis").
ALTER TABLE listings DROP CONSTRAINT listings_check2;
ALTER TABLE listings ADD CONSTRAINT listings_price_check
    CHECK ((is_free AND price IS NULL) OR (NOT is_free AND price > 0 AND (price_original IS NULL OR price_original >= price)));
