-- name: InsertListing :one
INSERT INTO listings (merchant_id, title, description, tier, unit, is_free, price_original, price, stock, halal, allergens,
                      made_at, storage, safety_checks, safe_until, pickup_start, pickup_end, location, video_media_id, idempotency_key)
SELECT m.id, @title, @description, @tier::listing_tier, @unit::quantity_unit, @is_free, sqlc.narg('price_original')::int, sqlc.narg('price')::int, @stock,
       coalesce(sqlc.narg('halal')::halal_status, m.halal), @allergens::allergen[],
       sqlc.narg('made_at')::timestamptz, @storage::storage_method, @safety_checks::safety_check[], sqlc.narg('safe_until')::timestamptz,
       @pickup_start, @pickup_end, m.location, sqlc.narg('video_media_id')::uuid, @idempotency_key
FROM merchants m WHERE m.id = @merchant_id
RETURNING id;

-- name: GetListingByIdempotency :one
SELECT id FROM listings WHERE merchant_id = $1 AND idempotency_key = $2;

-- name: AttachListingMedia :exec
INSERT INTO listing_media (listing_id, media_id, position) VALUES ($1, $2, $3);

-- name: GetOwnedListing :one
SELECT l.* FROM listings l JOIN merchants m ON m.id = l.merchant_id
WHERE l.id = $1 AND m.owner_id = $2 AND l.status <> 'deleted';

-- name: UpdateListing :exec
UPDATE listings SET
    stock          = coalesce(sqlc.narg('stock')::int, stock),
    price          = coalesce(sqlc.narg('price')::int, price),
    price_original = CASE WHEN @set_original::bool THEN sqlc.narg('price_original')::int ELSE price_original END,
    pickup_end     = coalesce(sqlc.narg('pickup_end')::timestamptz, pickup_end),
    status         = coalesce(sqlc.narg('status')::listing_status, status)
WHERE id = @id;

-- name: SoftDeleteListing :execrows
UPDATE listings SET status = 'deleted' WHERE id = $1 AND status <> 'deleted';

-- name: ListingPhotos :many
SELECT lm.listing_id, m.path, m.thumb_path
FROM listing_media lm JOIN media m ON m.id = lm.media_id
WHERE lm.listing_id = ANY(@ids::uuid[])
ORDER BY lm.listing_id, lm.position;

-- name: GetListingDetail :one
SELECT l.id, l.merchant_id, l.title, l.description, l.tier, l.unit, l.is_free, l.price_original, l.price, l.stock, l.sold_count,
       l.halal, l.allergens, l.made_at, l.storage, l.safety_checks, l.safe_until, l.pickup_start, l.pickup_end, l.status,
       m.name AS merchant_name, m.about AS merchant_about, m.address AS merchant_address, m.location_label,
       m.open_hours, m.halal AS merchant_halal, m.allergens AS merchant_allergens, m.rating_avg, m.rating_count, m.is_verified,
       m.delivery_available, m.prep_minutes, vm.path AS video_path,
       ST_Y(m.location::geometry)::float8 AS merchant_lat, ST_X(m.location::geometry)::float8 AS merchant_lng,
       coalesce(CASE WHEN sqlc.narg('lat')::float8 IS NOT NULL AND sqlc.narg('lng')::float8 IS NOT NULL
            THEN ST_Distance(l.location, ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography)
       END, -1)::float8 AS distance_m
FROM listings l JOIN merchants m ON m.id = l.merchant_id LEFT JOIN media vm ON vm.id = l.video_media_id
WHERE l.id = @id AND l.status <> 'deleted' AND m.status = 'approved';

-- name: ListFeed :many
-- Satu query untuk /home, /listings, dan halaman toko; urutan lain ditangani service. Syarat feed (CLAUDE.md aturan 4) selalu berlaku.
WITH feed AS (
    SELECT l.id, l.title, l.tier, l.unit, l.is_free, l.price, l.price_original, l.stock, l.sold_count, l.halal, l.allergens,
           l.pickup_start, l.pickup_end, l.safe_until, l.made_at, l.created_at,
           m.id AS merchant_id, m.name AS merchant_name, m.location_label, m.rating_avg, m.rating_count, m.is_verified,
           m.delivery_available, m.prep_minutes,
           coalesce((SELECT md.path FROM listing_media lm JOIN media md ON md.id = lm.media_id WHERE lm.listing_id = l.id ORDER BY lm.position LIMIT 1), '')::text AS cover_path,
           coalesce((SELECT md.thumb_path FROM listing_media lm JOIN media md ON md.id = lm.media_id WHERE lm.listing_id = l.id ORDER BY lm.position LIMIT 1), '')::text AS thumb_path,
           coalesce(CASE WHEN sqlc.narg('lat')::float8 IS NOT NULL AND sqlc.narg('lng')::float8 IS NOT NULL
                THEN ST_Distance(l.location, ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography)
           END, -1)::float8 AS distance_m,
           CASE WHEN l.is_free THEN 100
                WHEN l.price_original IS NOT NULL AND l.price_original > 0 THEN 100 - (l.price * 100 / l.price_original) -- bagi bulat seperti Listing.discountPercent di app Android
                ELSE 0 END::int AS discount_pct
    FROM listings l JOIN merchants m ON m.id = l.merchant_id
    WHERE l.status = 'active' AND l.stock > 0 AND now() BETWEEN l.pickup_start AND l.pickup_end AND m.status = 'approved'
      AND (sqlc.narg('tiers')::listing_tier[] IS NULL OR l.tier = ANY(sqlc.narg('tiers')::listing_tier[]))
      AND (sqlc.narg('q')::text IS NULL OR l.title ILIKE '%' || sqlc.narg('q')::text || '%')
      AND (sqlc.narg('only_free')::bool IS NOT TRUE OR l.is_free)
      AND (sqlc.narg('merchant_id')::uuid IS NULL OR l.merchant_id = sqlc.narg('merchant_id')::uuid)
      AND (sqlc.narg('exclude_id')::uuid IS NULL OR l.id <> sqlc.narg('exclude_id')::uuid)
      AND (sqlc.narg('max_km')::float8 IS NULL OR sqlc.narg('lat')::float8 IS NULL OR sqlc.narg('lng')::float8 IS NULL
           OR ST_DWithin(l.location, ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography, sqlc.narg('max_km')::float8 * 1000))
)
SELECT feed.*, count(*) OVER()::bigint AS total
FROM feed
-- urutan dasar = urutan daftar app Android (terbaru dulu); sort lain dilakukan di Go (rules Kotlin: HomeSort, CategorySort)
ORDER BY created_at DESC, id
LIMIT @lim OFFSET @off;

-- name: ListMerchantListings :many
SELECT l.id, l.title, l.tier, l.unit, l.is_free, l.price, l.price_original, l.stock, l.sold_count, l.status,
       l.pickup_start, l.pickup_end, l.safe_until,
       coalesce((SELECT md.path FROM listing_media lm JOIN media md ON md.id = lm.media_id WHERE lm.listing_id = l.id ORDER BY lm.position LIMIT 1), '')::text AS cover_path,
       coalesce((SELECT md.thumb_path FROM listing_media lm JOIN media md ON md.id = lm.media_id WHERE lm.listing_id = l.id ORDER BY lm.position LIMIT 1), '')::text AS thumb_path,
       count(*) OVER()::bigint AS total
FROM listings l
WHERE l.merchant_id = @merchant_id AND l.status <> 'deleted'
ORDER BY l.created_at DESC, l.id
LIMIT @lim OFFSET @off;

-- name: CountActiveListings :one
SELECT count(*) FROM listings
WHERE merchant_id = $1 AND status = 'active' AND stock > 0 AND now() BETWEEN pickup_start AND pickup_end;
