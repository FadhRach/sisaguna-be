CREATE TABLE media (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          media_kind NOT NULL,
    visibility    media_visibility NOT NULL DEFAULT 'public',
    mime          text NOT NULL,
    size_bytes    integer NOT NULL CHECK (size_bytes > 0),
    width         integer,
    height        integer,
    duration_ms   integer CHECK (duration_ms IS NULL OR duration_ms <= 30000),
    sha256        text NOT NULL,
    path          text NOT NULL,             -- relatif ke MEDIA_ROOT, mis. 'ab/ab12...ef.jpg'
    thumb_path    text,
    r2_key        text,
    r2_synced_at  timestamptz,
    attached_at   timestamptz,               -- NULL = belum dipakai listing/review/dll
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, sha256)
);
CREATE INDEX media_unsynced_idx ON media (created_at) WHERE r2_synced_at IS NULL;
CREATE INDEX media_orphan_idx   ON media (created_at) WHERE attached_at IS NULL;

ALTER TABLE users ADD CONSTRAINT users_photo_fk FOREIGN KEY (photo_media_id) REFERENCES media(id) ON DELETE SET NULL;
