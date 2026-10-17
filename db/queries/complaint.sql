-- name: InsertComplaint :one
INSERT INTO complaints (order_id, buyer_id, merchant_id, reason, detail)
VALUES (@order_id, @buyer_id, @merchant_id, @reason::complaint_reason, @detail)
RETURNING id, status, created_at;

-- name: AttachComplaintMedia :exec
INSERT INTO complaint_media (complaint_id, media_id, position) VALUES ($1, $2, $3);

-- name: ListComplaintsAdmin :many
SELECT c.id, c.order_id, c.reason, c.detail, c.status, c.resolution_note, c.created_at, c.resolved_at,
       o.code AS order_code, m.name AS merchant_name, u.name AS buyer_name, count(*) OVER()::bigint AS total
FROM complaints c
JOIN orders o ON o.id = c.order_id
JOIN merchants m ON m.id = c.merchant_id
JOIN users u ON u.id = c.buyer_id
WHERE sqlc.narg('status')::complaint_status IS NULL OR c.status = sqlc.narg('status')::complaint_status
ORDER BY c.created_at DESC, c.id
LIMIT @lim OFFSET @off;

-- name: ListComplaintMedia :many
SELECT complaint_id, media_id FROM complaint_media
WHERE complaint_id = ANY(@ids::uuid[]) ORDER BY complaint_id, position;

-- name: LockComplaint :one
SELECT id, order_id, buyer_id, status FROM complaints WHERE id = $1 FOR UPDATE;

-- name: UpdateComplaint :exec
UPDATE complaints SET
    status          = @status::complaint_status,
    resolution_note = coalesce(sqlc.narg('note'), resolution_note),
    resolved_at     = CASE WHEN @status::complaint_status IN ('resolved', 'rejected') THEN now() END
WHERE id = @id;

-- name: MerchantOwnsComplaintMedia :one
-- Pemilik toko yang dikomplain boleh melihat foto komplain.
SELECT EXISTS (
    SELECT 1 FROM complaint_media cm
    JOIN complaints c ON c.id = cm.complaint_id
    JOIN merchants m ON m.id = c.merchant_id
    WHERE cm.media_id = @media_id AND m.owner_id = @user_id
)::bool;
