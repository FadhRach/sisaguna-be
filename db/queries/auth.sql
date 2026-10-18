-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, family_id, token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRefreshTokenByHashForUpdate :one
SELECT * FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE;

-- name: RotateRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2
WHERE id = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: HasRecentReset :one
-- Jeda 60 detik per akun: mencegah membanjiri email pengguna.
SELECT EXISTS (SELECT 1 FROM password_resets WHERE user_id = $1 AND created_at > now() - interval '60 seconds')::bool;

-- name: InvalidateResets :exec
UPDATE password_resets SET used_at = now() WHERE user_id = $1 AND used_at IS NULL;

-- name: CreateReset :exec
INSERT INTO password_resets (user_id, code_hash, expires_at) VALUES ($1, $2, now() + interval '15 minutes');

-- name: GetActiveResetForUpdate :one
SELECT id, code_hash, attempts FROM password_resets
WHERE user_id = $1 AND used_at IS NULL AND expires_at > now()
ORDER BY created_at DESC LIMIT 1 FOR UPDATE;

-- name: BumpResetAttempts :exec
UPDATE password_resets SET attempts = attempts + 1 WHERE id = $1;

-- name: MarkResetUsed :exec
UPDATE password_resets SET used_at = now() WHERE id = $1;
