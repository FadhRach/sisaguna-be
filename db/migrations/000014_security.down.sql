DROP TABLE password_resets;
ALTER TABLE listings DROP COLUMN request_hash;
ALTER TABLE orders DROP COLUMN request_hash;
ALTER TABLE users DROP COLUMN sessions_valid_after;
