-- name: InsertReview :one
INSERT INTO reviews (order_id, merchant_id, buyer_id, stars, tags, comment)
VALUES (@order_id, @merchant_id, @buyer_id, @stars, @tags::text[], @comment)
RETURNING id, created_at;

-- name: AttachReviewMedia :exec
INSERT INTO review_media (review_id, media_id, position) VALUES ($1, $2, $3);

-- name: RecalcMerchantRating :exec
-- rating_avg dan rating_count selalu dihitung ulang dari tabel reviews, di transaksi yang sama dengan review baru.
UPDATE merchants SET
    rating_avg   = (SELECT round(avg(stars), 1) FROM reviews rv WHERE rv.merchant_id = $1),
    rating_count = (SELECT count(*) FROM reviews rv WHERE rv.merchant_id = $1)
WHERE id = $1;

-- name: ListReviews :many
SELECT r.id, r.stars, r.tags, r.comment, r.created_at, u.name AS buyer_name
FROM reviews r JOIN users u ON u.id = r.buyer_id
WHERE r.merchant_id = @merchant_id
  AND (sqlc.narg('cur_at')::timestamptz IS NULL OR (r.created_at, r.id) < (sqlc.narg('cur_at')::timestamptz, sqlc.narg('cur_id')::uuid))
ORDER BY r.created_at DESC, r.id DESC
LIMIT @lim;

-- name: ListReviewMedia :many
SELECT rm.review_id, md.path, md.thumb_path
FROM review_media rm JOIN media md ON md.id = rm.media_id
WHERE rm.review_id = ANY(@ids::uuid[])
ORDER BY rm.review_id, rm.position;
