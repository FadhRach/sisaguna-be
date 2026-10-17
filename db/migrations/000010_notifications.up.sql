CREATE TABLE notifications (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       notification_type NOT NULL,
    title      text NOT NULL,
    body       text NOT NULL,
    data       jsonb NOT NULL DEFAULT '{}',   -- mis. {"order_id": "..."} untuk deep link
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at    timestamptz,
    deleted_at timestamptz                    -- soft delete supaya tombol Batalkan bisa memulihkan
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE notification_prefs (
    user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled    boolean NOT NULL DEFAULT true,
    pickup     boolean NOT NULL DEFAULT true,
    "order"    boolean NOT NULL DEFAULT true,
    merchant   boolean NOT NULL DEFAULT true,
    promo      boolean NOT NULL DEFAULT false,
    impact     boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE device_tokens (  -- Fase 2, FCM
    token      text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform   text NOT NULL CHECK (platform IN ('android', 'ios')),
    updated_at timestamptz NOT NULL DEFAULT now()
);
