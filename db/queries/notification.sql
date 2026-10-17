-- name: EnsureNotificationPrefs :exec
INSERT INTO notification_prefs (user_id) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: CreateNotification :one
-- Tidak menyimpan apa pun kalau saklar utama atau saklar tipe itu dimatikan (ErrNoRows = dilewati).
INSERT INTO notifications (user_id, type, title, body, data)
SELECT @user_id, @type::notification_type, @title, @body, @data::jsonb
FROM notification_prefs p
WHERE p.user_id = @user_id AND p.enabled
  AND CASE @type::notification_type
        WHEN 'pickup'   THEN p.pickup
        WHEN 'order'    THEN p."order"
        WHEN 'merchant' THEN p.merchant
        WHEN 'promo'    THEN p.promo
        WHEN 'impact'   THEN p.impact
      END
RETURNING id, type, title;

-- name: ListNotifications :many
SELECT id, type, title, body, data, created_at, read_at
FROM notifications
WHERE user_id = @user_id AND deleted_at IS NULL
  AND (sqlc.narg('type')::notification_type IS NULL OR type = sqlc.narg('type'))
  AND (sqlc.narg('cur_at')::timestamptz IS NULL OR (created_at, id) < (sqlc.narg('cur_at')::timestamptz, sqlc.narg('cur_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = coalesce(read_at, now())
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL AND deleted_at IS NULL;

-- name: DeleteNotification :execrows
UPDATE notifications SET deleted_at = now() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: RestoreNotification :execrows
UPDATE notifications SET deleted_at = NULL WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL;

-- name: GetNotificationPrefs :one
SELECT enabled, pickup, "order", merchant, promo, impact FROM notification_prefs WHERE user_id = $1;

-- name: SetNotificationPrefs :exec
UPDATE notification_prefs SET enabled = @enabled, pickup = @pickup, "order" = @order_on, merchant = @merchant, promo = @promo, impact = @impact
WHERE user_id = @user_id;

-- name: UpsertDeviceToken :exec
-- Token milik perangkat; kalau perangkat berpindah akun, pemiliknya ikut berpindah.
INSERT INTO device_tokens (token, user_id, platform) VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now();

-- name: DeleteDeviceToken :exec
DELETE FROM device_tokens WHERE token = $1 AND user_id = $2;
