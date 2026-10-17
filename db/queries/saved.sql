-- name: SaveMerchant :exec
INSERT INTO saved_merchants (user_id, merchant_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: UnsaveMerchant :exec
DELETE FROM saved_merchants WHERE user_id = $1 AND merchant_id = $2;

-- name: IsMerchantSaved :one
SELECT EXISTS (SELECT 1 FROM saved_merchants WHERE user_id = $1 AND merchant_id = $2)::bool;

-- name: ListSavedMerchants :many
-- Urutan = urutan disimpan (tertua dulu), seperti SavedMerchantRepository di app Android.
WITH avail AS (
    SELECT merchant_id, count(*) AS n, min(pickup_end) AS next_end
    FROM listings WHERE status = 'active' AND stock > 0 AND now() BETWEEN pickup_start AND pickup_end
    GROUP BY merchant_id
)
SELECT m.id, m.name, m.location_label, m.todays_offer, m.rating_avg, m.rating_count, m.is_verified, s.created_at AS saved_at,
       bm.path AS banner_path, coalesce(a.n, 0)::bigint AS available_count, a.next_end::timestamptz AS next_pickup_end,
       coalesce(CASE WHEN sqlc.narg('lat')::float8 IS NOT NULL AND sqlc.narg('lng')::float8 IS NOT NULL
                     THEN ST_Distance(m.location, ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography)
                END, -1)::float8 AS distance_m
FROM saved_merchants s
JOIN merchants m ON m.id = s.merchant_id
LEFT JOIN avail a ON a.merchant_id = m.id
LEFT JOIN media bm ON bm.id = m.banner_media_id
WHERE s.user_id = @user_id AND m.status = 'approved'
ORDER BY s.created_at ASC, m.id;

-- name: UserImpact :one
-- Tanpa baris (belum ada order selesai) dijawab ErrNoRows -> semua angka nol.
SELECT completed_orders::bigint, portions::bigint, feed_compost_kg::bigint, saved_rupiah::bigint FROM user_impact WHERE user_id = $1;

-- name: ListMerchantSavers :many
SELECT user_id FROM saved_merchants WHERE merchant_id = $1;
