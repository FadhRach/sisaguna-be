ALTER TABLE listings DROP CONSTRAINT listings_price_check;
UPDATE listings SET price_original = NULL WHERE is_free;
ALTER TABLE listings ADD CONSTRAINT listings_check2
    CHECK ((is_free AND price IS NULL AND price_original IS NULL) OR (NOT is_free AND price > 0 AND (price_original IS NULL OR price_original >= price)));

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
SELECT o.buyer_id AS user_id,
       o.completed_orders,
       coalesce(i.portions, 0)                           AS portions,
       coalesce(i.feed_compost_kg, 0)                    AS feed_compost_kg,
       coalesce(i.item_savings, 0) + o.voucher_savings   AS saved_rupiah
FROM per_order o
LEFT JOIN per_item i USING (buyer_id);

DROP TABLE payment_methods;
ALTER TABLE users DROP COLUMN impact_base_portions, DROP COLUMN impact_base_feed_kg, DROP COLUMN impact_base_saved_rupiah;
ALTER TABLE vouchers DROP COLUMN applies_to_label;
ALTER TABLE orders DROP COLUMN delivery_discount;
