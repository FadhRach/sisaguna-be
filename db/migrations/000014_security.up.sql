-- Token akses yang diterbitkan sebelum waktu ini ditolak (ganti/reset password mencabut semua sesi).
ALTER TABLE users ADD COLUMN sessions_valid_after timestamptz NOT NULL DEFAULT '1970-01-01';

-- Hash isi request untuk Idempotency-Key: key yang sama dengan isi berbeda ditolak (NULL = baris lama, tidak dibandingkan).
ALTER TABLE orders   ADD COLUMN request_hash text;
ALTER TABLE listings ADD COLUMN request_hash text;

-- Kode 6 digit untuk lupa password (disimpan sebagai hash, berlaku singkat, dibatasi percobaan).
CREATE TABLE password_resets (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  text NOT NULL,
    attempts   smallint NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX password_resets_user_idx ON password_resets (user_id, created_at DESC);
