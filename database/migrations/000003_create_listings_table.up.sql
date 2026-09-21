CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE listings (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mitra_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title            text NOT NULL,
    description      text,
    tier             text NOT NULL,
    quantity         numeric NOT NULL,
    unit             text NOT NULL,
    price            numeric NOT NULL DEFAULT 0,
    photo_url        text,
    pickup_location  geography(Point,4326) NOT NULL,
    pickup_address   text NOT NULL,
    ready_at         timestamptz NOT NULL,
    pickup_deadline  timestamptz NOT NULL,
    status           text NOT NULL DEFAULT 'available',
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_listings_pickup_location ON listings USING GIST (pickup_location);
CREATE INDEX idx_listings_status_deadline ON listings (status, pickup_deadline);
