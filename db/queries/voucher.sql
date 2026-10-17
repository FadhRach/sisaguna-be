-- name: GetVoucherState :one
-- valid dihitung dengan jam database (satu sumber waktu), dipakai saat klaim dan checkout.
SELECT sqlc.embed(v),
       coalesce(v.is_active AND v.starts_at <= now() AND v.expires_at > now() AND (v.quota IS NULL OR v.used_count < v.quota), false)::bool AS valid
FROM vouchers v WHERE v.code = $1;

-- name: ClaimVoucher :execrows
INSERT INTO user_vouchers (user_id, voucher_code) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListUserVouchers :many
SELECT v.code, v.title, v.description, v.percent_off, v.flat_off, v.max_discount, v.min_spend, v.merchant_id,
       v.applies_to_tier, v.applies_to_label, v.expires_at, v.terms, v.accent_hex, im.path AS image_path, mm.name AS merchant_name,
       (uv.user_id IS NOT NULL) AS claimed, uv.used_at
FROM vouchers v
LEFT JOIN media im ON im.id = v.image_media_id
LEFT JOIN merchants mm ON mm.id = v.merchant_id
LEFT JOIN user_vouchers uv ON uv.voucher_code = v.code AND uv.user_id = @user_id
WHERE (v.is_active AND v.starts_at <= now() AND v.expires_at > now() AND (v.quota IS NULL OR v.used_count < v.quota))
   OR uv.user_id IS NOT NULL
ORDER BY (uv.used_at IS NOT NULL), v.expires_at, v.code;

-- name: GetUserVoucherForUpdate :one
SELECT * FROM user_vouchers WHERE user_id = $1 AND voucher_code = $2 FOR UPDATE;

-- name: ConsumeVoucherQuota :execrows
UPDATE vouchers SET used_count = used_count + 1
WHERE code = $1 AND is_active AND starts_at <= now() AND expires_at > now() AND (quota IS NULL OR used_count < quota);

-- name: MarkVoucherUsed :execrows
UPDATE user_vouchers SET used_at = now(), order_id = $3
WHERE user_id = $1 AND voucher_code = $2 AND used_at IS NULL;

-- name: ReleaseVoucherOfOrder :one
-- Mengembalikan voucher yang dipakai order (used_at NULL) dan mengembalikan kodenya; ErrNoRows = tidak ada voucher.
UPDATE user_vouchers SET used_at = NULL, order_id = NULL
WHERE order_id = $1 AND used_at IS NOT NULL
RETURNING voucher_code;

-- name: DecrementVoucherUsage :exec
UPDATE vouchers SET used_count = greatest(used_count - 1, 0) WHERE code = $1;

-- name: CreateVoucher :exec
INSERT INTO vouchers (code, title, description, percent_off, flat_off, max_discount, min_spend, merchant_id, applies_to_tier, applies_to_label, image_media_id,
                      starts_at, expires_at, quota, terms, accent_hex)
VALUES (@code, @title, @description, sqlc.narg('percent_off')::smallint, sqlc.narg('flat_off')::int, sqlc.narg('max_discount')::int, @min_spend,
        sqlc.narg('merchant_id')::uuid, sqlc.narg('applies_to_tier')::listing_tier, @applies_to_label, sqlc.narg('image_media_id')::uuid,
        coalesce(sqlc.narg('starts_at')::timestamptz, now()), @expires_at, sqlc.narg('quota')::int, @terms::text[], @accent_hex);

-- name: ListVouchersAdmin :many
SELECT sqlc.embed(v), count(*) OVER()::bigint AS total FROM vouchers v ORDER BY v.created_at DESC, v.code LIMIT @lim OFFSET @off;

-- name: UpdateVoucher :exec
UPDATE vouchers SET
    title       = coalesce(sqlc.narg('title'), title),
    description = coalesce(sqlc.narg('description'), description),
    min_spend   = coalesce(sqlc.narg('min_spend')::int, min_spend),
    expires_at  = coalesce(sqlc.narg('expires_at')::timestamptz, expires_at),
    quota       = coalesce(sqlc.narg('quota')::int, quota),
    is_active   = coalesce(sqlc.narg('is_active')::bool, is_active),
    terms       = coalesce(sqlc.narg('terms')::text[], terms),
    accent_hex  = coalesce(sqlc.narg('accent_hex'), accent_hex),
    applies_to_label = coalesce(sqlc.narg('applies_to_label'), applies_to_label),
    image_media_id = coalesce(sqlc.narg('image_media_id')::uuid, image_media_id)
WHERE code = @code;
