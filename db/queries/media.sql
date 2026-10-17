-- name: UpsertMedia :one
-- Upload ulang file yang sama persis oleh pemilik yang sama mengembalikan baris lama.
INSERT INTO media (owner_id, kind, visibility, mime, size_bytes, width, height, sha256, path, thumb_path)
VALUES ($1, 'image', $2, 'image/jpeg', $3, $4, $5, $6, $7, $8)
ON CONFLICT (owner_id, sha256) DO UPDATE SET sha256 = EXCLUDED.sha256
RETURNING *;

-- name: UpsertVideo :one
INSERT INTO media (owner_id, kind, visibility, mime, size_bytes, duration_ms, sha256, path)
VALUES ($1, 'video', 'public', 'video/mp4', $2, $3, $4, $5)
ON CONFLICT (owner_id, sha256) DO UPDATE SET sha256 = EXCLUDED.sha256
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = $1;

-- name: AttachMedia :exec
UPDATE media SET attached_at = now() WHERE id = $1 AND attached_at IS NULL;

-- name: DetachMedia :exec
UPDATE media SET attached_at = NULL WHERE id = $1;

-- name: ListUnsyncedMedia :many
SELECT id, visibility, path, thumb_path FROM media WHERE r2_synced_at IS NULL ORDER BY created_at LIMIT 100;

-- name: MarkMediaSynced :exec
UPDATE media SET r2_synced_at = now() WHERE id = $1;
