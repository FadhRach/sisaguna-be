-- name: CreateUser :one
INSERT INTO users (name, email, phone, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications
WHERE user_id = $1 AND read_at IS NULL AND deleted_at IS NULL;

-- name: GetProfile :one
SELECT u.*, m.path AS photo_path
FROM users u
LEFT JOIN media m ON m.id = u.photo_media_id
WHERE u.id = $1;

-- name: UpdateUserProfile :exec
UPDATE users SET
    name           = coalesce(sqlc.narg('name'), name),
    email          = coalesce(sqlc.narg('email'), email),
    phone          = coalesce(sqlc.narg('phone'), phone),
    avatar_key     = coalesce(sqlc.narg('avatar_key'), avatar_key),
    photo_media_id = CASE WHEN sqlc.arg('set_photo')::bool THEN sqlc.narg('photo_media_id')::uuid ELSE photo_media_id END
WHERE id = $1;

-- name: UpdatePasswordHash :exec
-- sessions_valid_after = sekarang: semua access token lama ikut tidak berlaku.
UPDATE users SET password_hash = $2, sessions_valid_after = now() WHERE id = $1;

-- name: SetSelectedAddress :exec
UPDATE users SET selected_address_id = $2 WHERE id = $1;

-- name: GetAuthState :one
-- Dicek middleware (dengan cache singkat) supaya akun yang dihapus / sesi yang dicabut tidak berlaku sampai token kedaluwarsa.
SELECT is_admin, sessions_valid_after FROM users WHERE id = $1;
