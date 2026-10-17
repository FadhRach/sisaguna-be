-- name: LockListingsForCheckout :many
SELECT * FROM listings WHERE id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;

-- name: GetMerchantBrief :one
SELECT id, owner_id, status, name FROM merchants WHERE id = $1;

-- name: GetListingsByIDs :many
SELECT * FROM listings WHERE id = ANY(@ids::uuid[]);

-- name: DecrementStock :one
-- Mengembalikan stok baru; ErrNoRows = stok tidak cukup (out_of_stock).
UPDATE listings SET stock = stock - @qty, sold_count = sold_count + @qty
WHERE id = @id AND stock >= @qty
RETURNING stock;

-- name: RestoreStock :one
-- Stok kembali hanya kalau listing masih aktif dan jendela pickup belum lewat (ErrNoRows = tidak dikembalikan).
UPDATE listings SET stock = stock + @qty, sold_count = greatest(sold_count - @qty, 0)
WHERE id = @id AND status = 'active' AND pickup_end > now()
RETURNING stock;

-- name: InsertOrder :one
INSERT INTO orders (code, buyer_id, merchant_id, status, fulfillment, payment_kind, payment_status, paid_at, note, subtotal, original_total,
                    voucher_code, voucher_discount, service_fee, delivery_fee, delivery_discount, total, commission, merchant_payout,
                    delivery_courier, delivery_speed, delivery_eta_min, delivery_eta_max, delivery_address,
                    pickup_by, pickup_end, idempotency_key, voucher_funded_by)
VALUES (@code, @buyer_id, @merchant_id, 'ready', @fulfillment::fulfillment, @payment_kind::payment_kind, @payment_status::payment_status,
        CASE WHEN @payment_status::payment_status = 'paid' THEN now() END, @note, @subtotal, @original_total,
        sqlc.narg('voucher_code'), @voucher_discount, @service_fee, @delivery_fee, @delivery_discount, @total, @commission, @merchant_payout,
        sqlc.narg('delivery_courier')::courier, sqlc.narg('delivery_speed')::delivery_speed, sqlc.narg('delivery_eta_min')::smallint, sqlc.narg('delivery_eta_max')::smallint,
        sqlc.narg('delivery_address')::jsonb,
        sqlc.narg('pickup_by')::timestamptz, @pickup_end, @idempotency_key, sqlc.narg('voucher_funded_by'))
RETURNING id;

-- name: GetDeliveryTarget :one
-- Alamat milik pembeli + jarak lurus ke toko (PostGIS) + pengaturan pengiriman toko.
SELECT a.id, a.label, a.full_address, a.note,
       ST_Y(a.location::geometry)::float8 AS lat, ST_X(a.location::geometry)::float8 AS lng,
       (ST_Distance(a.location, m.location) / 1000.0)::float8 AS distance_km,
       m.delivery_available, m.prep_minutes
FROM addresses a, merchants m
WHERE a.id = @address_id AND a.user_id = @user_id AND m.id = @merchant_id;

-- name: GetSelectedAddressID :one
SELECT selected_address_id FROM users WHERE id = $1;

-- name: InsertOrderItem :exec
INSERT INTO order_items (order_id, listing_id, title, tier, unit, image_media_id, quantity, unit_price, unit_original_price)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetOrderByIdempotency :one
SELECT id FROM orders WHERE buyer_id = $1 AND idempotency_key = $2;

-- name: GetOrderView :one
SELECT sqlc.embed(o), m.name AS merchant_name, m.owner_id AS merchant_owner_id, u.name AS buyer_name,
       m.location_label AS merchant_location, ST_Y(m.location::geometry)::float8 AS merchant_lat, ST_X(m.location::geometry)::float8 AS merchant_lng,
       EXISTS (SELECT 1 FROM reviews r WHERE r.order_id = o.id)::bool AS has_review,
       coalesce((SELECT c.status::text FROM complaints c WHERE c.order_id = o.id), '')::text AS complaint_status
FROM orders o JOIN merchants m ON m.id = o.merchant_id JOIN users u ON u.id = o.buyer_id
WHERE o.id = $1;

-- name: ListOrderItems :many
SELECT oi.order_id, oi.listing_id, oi.title, oi.tier, oi.unit, oi.quantity, oi.unit_price, oi.unit_original_price, md.path AS image_path, md.thumb_path
FROM order_items oi LEFT JOIN media md ON md.id = oi.image_media_id
WHERE oi.order_id = ANY(@ids::uuid[])
ORDER BY oi.order_id, oi.title;

-- name: LockOrder :one
SELECT * FROM orders WHERE id = $1 FOR UPDATE;

-- name: SetOrderCancelled :exec
UPDATE orders SET status = @status::order_status, cancelled_at = now(),
    payment_status = CASE WHEN payment_status = 'paid' AND payment_kind <> 'cash' THEN 'refunded'::payment_status ELSE payment_status END, cancelled_by = sqlc.narg('cancelled_by')::chat_side, cancel_reason = sqlc.narg('reason')
WHERE id = @id;

-- name: SetOrderCompleted :exec
UPDATE orders SET status = 'completed', payment_status = 'paid', paid_at = coalesce(paid_at, now()), completed_at = now() WHERE id = $1;

-- name: IncMerchantRescued :exec
UPDATE merchants SET total_rescued = total_rescued + $2 WHERE id = $1;

-- name: CountRecentWrongCodes :one
SELECT count(*) FROM order_code_attempts WHERE order_id = $1 AND attempted_at > now() - interval '10 minutes';

-- name: AddWrongCodeAttempt :exec
INSERT INTO order_code_attempts (order_id) VALUES ($1);

-- name: ListBuyerOrders :many
SELECT sqlc.embed(o), m.name AS merchant_name, m.owner_id AS merchant_owner_id, u.name AS buyer_name,
       m.location_label AS merchant_location, ST_Y(m.location::geometry)::float8 AS merchant_lat, ST_X(m.location::geometry)::float8 AS merchant_lng,
       EXISTS (SELECT 1 FROM reviews r WHERE r.order_id = o.id)::bool AS has_review,
       coalesce((SELECT c.status::text FROM complaints c WHERE c.order_id = o.id), '')::text AS complaint_status
FROM orders o JOIN merchants m ON m.id = o.merchant_id JOIN users u ON u.id = o.buyer_id
WHERE o.buyer_id = @buyer_id AND o.status = ANY(@statuses::order_status[])
  AND (sqlc.narg('cur_at')::timestamptz IS NULL OR (o.created_at, o.id) < (sqlc.narg('cur_at')::timestamptz, sqlc.narg('cur_id')::uuid))
ORDER BY o.created_at DESC, o.id DESC
LIMIT @lim;

-- name: ListMerchantOrders :many
SELECT sqlc.embed(o), m.name AS merchant_name, m.owner_id AS merchant_owner_id, u.name AS buyer_name,
       m.location_label AS merchant_location, ST_Y(m.location::geometry)::float8 AS merchant_lat, ST_X(m.location::geometry)::float8 AS merchant_lng,
       EXISTS (SELECT 1 FROM reviews r WHERE r.order_id = o.id)::bool AS has_review,
       coalesce((SELECT c.status::text FROM complaints c WHERE c.order_id = o.id), '')::text AS complaint_status
FROM orders o JOIN merchants m ON m.id = o.merchant_id JOIN users u ON u.id = o.buyer_id
WHERE o.merchant_id = @merchant_id AND o.status = ANY(@statuses::order_status[])
  AND (sqlc.narg('cur_at')::timestamptz IS NULL OR (o.created_at, o.id) < (sqlc.narg('cur_at')::timestamptz, sqlc.narg('cur_id')::uuid))
ORDER BY o.created_at DESC, o.id DESC
LIMIT @lim;

-- name: MerchantDashboard :one
-- "Hari ini" mengikuti zona Asia/Jakarta.
SELECT
    coalesce(sum(subtotal - voucher_discount) FILTER (WHERE status = 'completed' AND (completed_at AT TIME ZONE 'Asia/Jakarta')::date = (now() AT TIME ZONE 'Asia/Jakarta')::date), 0)::bigint AS sales_today,
    coalesce(sum(commission) FILTER (WHERE status = 'completed' AND (completed_at AT TIME ZONE 'Asia/Jakarta')::date = (now() AT TIME ZONE 'Asia/Jakarta')::date), 0)::bigint AS commission_today,
    coalesce(sum(merchant_payout) FILTER (WHERE status = 'completed' AND (completed_at AT TIME ZONE 'Asia/Jakarta')::date = (now() AT TIME ZONE 'Asia/Jakarta')::date), 0)::bigint AS net_today,
    count(*) FILTER (WHERE status = 'completed' AND (completed_at AT TIME ZONE 'Asia/Jakarta')::date = (now() AT TIME ZONE 'Asia/Jakarta')::date)::bigint AS completed_today,
    count(*) FILTER (WHERE status = 'ready')::bigint AS pending_orders
FROM orders WHERE merchant_id = $1;

-- name: ListNoShowCandidates :many
-- Pengiriman tidak punya pickup_by: dianggap tak terselesaikan 2 jam setelah jendela listing tutup.
SELECT id FROM orders WHERE status = 'ready' AND coalesce(pickup_by, pickup_end + interval '2 hours') + interval '15 minutes' < now()
ORDER BY coalesce(pickup_by, pickup_end + interval '2 hours') LIMIT 100;

-- name: ListReminderCandidates :many
SELECT id FROM orders WHERE status = 'ready' AND reminded_at IS NULL AND pickup_by > now() AND pickup_by - now() < interval '30 minutes' ORDER BY pickup_by LIMIT 100;

-- name: MarkReminded :exec
UPDATE orders SET reminded_at = now() WHERE id = $1;

-- name: ListOrphanMedia :many
-- File yatim: tidak pernah dipakai dan lebih dari 24 jam. shared_path = file fisik juga dipakai baris media lain.
SELECT m.id, m.visibility, m.path, m.thumb_path,
       EXISTS (SELECT 1 FROM media o WHERE o.path = m.path AND o.id <> m.id) AS shared_path
FROM media m WHERE m.attached_at IS NULL AND m.created_at < now() - interval '24 hours' LIMIT 200;

-- name: DeleteMedia :exec
DELETE FROM media WHERE id = $1;

-- name: GetMediaIDByListingCover :one
SELECT lm.media_id FROM listing_media lm WHERE lm.listing_id = $1 ORDER BY lm.position LIMIT 1;

-- name: ListOrderReviews :many
SELECT order_id, stars, tags, comment, created_at FROM reviews WHERE order_id = ANY(@ids::uuid[]);

-- name: ListOrderComplaints :many
SELECT id, order_id, reason, detail, status, resolution_note, created_at FROM complaints WHERE order_id = ANY(@ids::uuid[]);
