-- name: CreateMerchant :one
INSERT INTO merchants (owner_id, name, about, address, location, location_label, open_hours, halal, allergens, prep_minutes)
VALUES (@owner_id, @name, @about, @address, ST_SetSRID(ST_MakePoint(@lng::float8, @lat::float8), 4326)::geography,
        @location_label, @open_hours, @halal::halal_status, @allergens::allergen[], @prep_minutes)
RETURNING id;

-- name: GetMerchantByOwner :one
SELECT m.id, m.owner_id, m.name, m.status, m.is_verified, m.about, m.address, m.location_label, m.open_hours,
       m.halal, m.allergens, m.todays_offer, m.delivery_available, m.prep_minutes,
       m.rating_avg, m.rating_count, m.total_rescued, m.created_at, bm.path AS banner_path,
       ST_Y(m.location::geometry)::float8 AS lat, ST_X(m.location::geometry)::float8 AS lng
FROM merchants m LEFT JOIN media bm ON bm.id = m.banner_media_id WHERE m.owner_id = $1;

-- name: GetApprovedMerchant :one
SELECT m.id, m.owner_id, m.name, m.status, m.is_verified, m.about, m.address, m.location_label, m.open_hours,
       m.halal, m.allergens, m.todays_offer, m.delivery_available, m.prep_minutes,
       m.rating_avg, m.rating_count, m.total_rescued, m.created_at, bm.path AS banner_path,
       ST_Y(m.location::geometry)::float8 AS lat, ST_X(m.location::geometry)::float8 AS lng,
       coalesce(CASE WHEN sqlc.narg('user_lat')::float8 IS NOT NULL AND sqlc.narg('user_lng')::float8 IS NOT NULL
            THEN ST_Distance(m.location, ST_SetSRID(ST_MakePoint(sqlc.narg('user_lng')::float8, sqlc.narg('user_lat')::float8), 4326)::geography)
       END, -1)::float8 AS distance_m
FROM merchants m LEFT JOIN media bm ON bm.id = m.banner_media_id WHERE m.id = @id AND m.status = 'approved';

-- name: UpdateMerchant :exec
UPDATE merchants SET
    name           = coalesce(sqlc.narg('name'), name),
    about          = coalesce(sqlc.narg('about'), about),
    address        = coalesce(sqlc.narg('address'), address),
    location_label = coalesce(sqlc.narg('location_label'), location_label),
    open_hours     = coalesce(sqlc.narg('open_hours'), open_hours),
    halal          = coalesce(sqlc.narg('halal')::halal_status, halal),
    allergens      = coalesce(sqlc.narg('allergens')::allergen[], allergens),
    todays_offer   = CASE WHEN @set_offer::bool THEN sqlc.narg('todays_offer') ELSE todays_offer END,
    prep_minutes   = coalesce(sqlc.narg('prep_minutes')::smallint, prep_minutes),
    banner_media_id = CASE WHEN @set_banner::bool THEN sqlc.narg('banner_media_id')::uuid ELSE banner_media_id END,
    location       = CASE WHEN sqlc.narg('lat')::float8 IS NOT NULL AND sqlc.narg('lng')::float8 IS NOT NULL
                          THEN ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography
                          ELSE location END
WHERE id = @id;

-- name: SyncListingsLocation :exec
-- Lokasi listing adalah salinan lokasi toko; ikut diperbarui saat toko pindah.
UPDATE listings SET location = (SELECT mm.location FROM merchants mm WHERE mm.id = $1)
WHERE merchant_id = $1 AND status <> 'deleted';

-- name: ListOfferMerchants :many
WITH near AS (
    SELECT m.id, m.name, m.location_label, m.todays_offer, m.rating_avg, m.rating_count, m.is_verified,
           coalesce(CASE WHEN sqlc.narg('lat')::float8 IS NOT NULL AND sqlc.narg('lng')::float8 IS NOT NULL
                         THEN ST_Distance(m.location, ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography)
                    END, -1)::float8 AS distance_m
    FROM merchants m
    WHERE m.status = 'approved' AND m.todays_offer IS NOT NULL AND m.todays_offer <> ''
      AND (sqlc.narg('exclude_owner')::uuid IS NULL OR m.owner_id <> sqlc.narg('exclude_owner')::uuid)
      AND (sqlc.narg('max_km')::float8 IS NULL OR sqlc.narg('lat')::float8 IS NULL OR sqlc.narg('lng')::float8 IS NULL
           OR ST_DWithin(m.location, ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography, sqlc.narg('max_km')::float8 * 1000))
)
SELECT * FROM near
ORDER BY CASE WHEN distance_m >= 0 THEN distance_m END ASC NULLS LAST, rating_avg DESC NULLS LAST, id
LIMIT @lim;

-- name: ListMerchantsForAdmin :many
SELECT m.id, m.name, m.status, m.address, m.created_at, u.name AS owner_name, u.email AS owner_email, count(*) OVER()::bigint AS total
FROM merchants m JOIN users u ON u.id = m.owner_id
WHERE sqlc.narg('status')::merchant_status IS NULL OR m.status = sqlc.narg('status')::merchant_status
ORDER BY m.created_at DESC, m.id
LIMIT @lim OFFSET @off;

-- name: SetMerchantStatus :one
UPDATE merchants SET status = @status::merchant_status, is_verified = (@status::merchant_status = 'approved')
WHERE id = @id
RETURNING owner_id, name;

-- name: GetMerchantBanner :one
SELECT banner_media_id FROM merchants WHERE id = $1;

-- name: ListMerchantPhotos :many
SELECT mp.merchant_id, md.path, md.thumb_path
FROM merchant_photos mp JOIN media md ON md.id = mp.media_id
WHERE mp.merchant_id = ANY(@ids::uuid[])
ORDER BY mp.merchant_id, mp.position;

-- name: DeleteMerchantPhotos :many
DELETE FROM merchant_photos WHERE merchant_id = $1 RETURNING media_id;

-- name: InsertMerchantPhoto :exec
INSERT INTO merchant_photos (merchant_id, media_id, position) VALUES ($1, $2, $3);

-- name: MerchantRatingBreakdown :many
SELECT stars::int AS stars, count(*)::int AS n FROM reviews WHERE merchant_id = $1 GROUP BY stars;

-- name: ListApprovedMerchants :many
-- Daftar toko untuk rekomendasi penyedia: jumlah listing yang tampil sekarang, pickup paling awal, jarak, dan apakah viewer menyimpannya.
WITH avail AS (
    SELECT merchant_id, count(*) AS n, min(pickup_end) AS next_end
    FROM listings WHERE status = 'active' AND stock > 0 AND now() BETWEEN pickup_start AND pickup_end
    GROUP BY merchant_id
), rows AS (
    SELECT m.id, m.name, m.location_label, m.todays_offer, m.rating_avg, m.rating_count, m.is_verified, bm.path AS banner_path,
           coalesce(a.n, 0)::bigint AS available_count, a.next_end::timestamptz AS next_pickup_end,
           coalesce(CASE WHEN sqlc.narg('lat')::float8 IS NOT NULL AND sqlc.narg('lng')::float8 IS NOT NULL
                         THEN ST_Distance(m.location, ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography)
                    END, -1)::float8 AS distance_m,
           (sqlc.narg('viewer')::uuid IS NOT NULL AND EXISTS (SELECT 1 FROM saved_merchants s WHERE s.user_id = sqlc.narg('viewer')::uuid AND s.merchant_id = m.id))::bool AS is_saved
    FROM merchants m
    LEFT JOIN avail a ON a.merchant_id = m.id
    LEFT JOIN media bm ON bm.id = m.banner_media_id
    WHERE m.status = 'approved'
      AND (sqlc.narg('viewer')::uuid IS NULL OR m.owner_id <> sqlc.narg('viewer')::uuid)
      AND (NOT @available_only::bool OR a.n > 0)
)
SELECT * FROM rows ORDER BY CASE WHEN distance_m >= 0 THEN distance_m END ASC NULLS LAST, name, id LIMIT @lim;

-- name: ListAdminIDs :many
SELECT id FROM users WHERE is_admin;
