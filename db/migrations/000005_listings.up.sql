CREATE TABLE listings (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     uuid NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    title           text NOT NULL CHECK (char_length(title) BETWEEN 3 AND 80),
    description     text NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    tier            listing_tier NOT NULL,
    unit            quantity_unit NOT NULL,
    is_free         boolean NOT NULL DEFAULT false,
    price_original  integer CHECK (price_original IS NULL OR price_original > 0),
    price           integer,
    stock           integer NOT NULL CHECK (stock >= 0),
    sold_count      integer NOT NULL DEFAULT 0,
    halal           halal_status NOT NULL DEFAULT 'unverified',
    allergens       allergen[] NOT NULL DEFAULT '{}',
    made_at         timestamptz,
    storage         storage_method NOT NULL DEFAULT 'room_temp',
    safety_checks   safety_check[] NOT NULL DEFAULT '{}',
    safe_until      timestamptz,             -- dihitung server: made_at + jam aman storage
    pickup_start    timestamptz NOT NULL DEFAULT now(),
    pickup_end      timestamptz NOT NULL,
    location        geography(Point, 4326) NOT NULL,  -- salinan lokasi toko untuk query feed
    video_media_id  uuid REFERENCES media(id) ON DELETE SET NULL,
    status          listing_status NOT NULL DEFAULT 'active',
    idempotency_key text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, idempotency_key),
    CHECK (pickup_end > pickup_start),
    CHECK (unit = CASE WHEN tier = 'human' THEN 'portion'::quantity_unit ELSE 'kg'::quantity_unit END),
    CHECK ((is_free AND price IS NULL AND price_original IS NULL)
        OR (NOT is_free AND price > 0 AND (price_original IS NULL OR price_original >= price)))
);
CREATE INDEX listings_feed_geo_idx ON listings USING gist (location) WHERE status = 'active' AND stock > 0;
CREATE INDEX listings_feed_time_idx ON listings (tier, pickup_end) WHERE status = 'active' AND stock > 0;
CREATE INDEX listings_merchant_idx ON listings (merchant_id, created_at DESC);
CREATE INDEX listings_title_trgm_idx ON listings USING gin (title gin_trgm_ops);
CREATE TRIGGER listings_updated_at BEFORE UPDATE ON listings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE listing_media (
    listing_id uuid NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    media_id   uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    position   smallint NOT NULL CHECK (position BETWEEN 0 AND 4),  -- maks 5 foto, posisi 0 = cover
    PRIMARY KEY (listing_id, media_id),
    UNIQUE (listing_id, position)
);
