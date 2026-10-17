CREATE TABLE users (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    email               citext NOT NULL UNIQUE,
    phone               text NOT NULL CHECK (phone ~ '^[0-9]{8,14}$'),  -- digit tanpa +62, sama seperti Android
    password_hash       text NOT NULL,
    is_admin            boolean NOT NULL DEFAULT false,
    avatar_key          text,                 -- avatar buah, dipakai kalau tidak ada foto
    photo_media_id      uuid,                 -- FK ditambah setelah tabel media ada
    location_label      text,
    selected_address_id uuid,                 -- FK ditambah setelah tabel addresses ada
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE refresh_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id    uuid NOT NULL,              -- semua token hasil rotasi dari satu login
    token_hash   text NOT NULL UNIQUE,
    replaced_by  uuid REFERENCES refresh_tokens(id),
    user_agent   text,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_idx   ON refresh_tokens (user_id);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
