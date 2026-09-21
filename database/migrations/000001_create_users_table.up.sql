CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name           text NOT NULL,
    email          text NOT NULL UNIQUE,
    phone          text NOT NULL,
    password_hash  text NOT NULL,
    roles          text[] NOT NULL DEFAULT '{}',
    business_name  text,
    avg_rating     numeric(2,1),
    created_at     timestamptz NOT NULL DEFAULT now()
);
