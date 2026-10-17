CREATE TABLE merchants (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id           uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    name               text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 60),
    status             merchant_status NOT NULL DEFAULT 'pending',
    is_verified        boolean NOT NULL DEFAULT false,
    about              text NOT NULL DEFAULT '',
    address            text NOT NULL,
    location           geography(Point, 4326) NOT NULL,
    location_label     text NOT NULL DEFAULT '',
    open_hours         text NOT NULL DEFAULT '',
    halal              halal_status NOT NULL DEFAULT 'unverified',
    allergens          allergen[] NOT NULL DEFAULT '{}',
    todays_offer       text,
    delivery_available boolean NOT NULL DEFAULT false,
    prep_minutes       smallint NOT NULL DEFAULT 15 CHECK (prep_minutes BETWEEN 0 AND 240),
    banner_media_id    uuid REFERENCES media(id) ON DELETE SET NULL,
    rating_avg         numeric(2,1),
    rating_count       integer NOT NULL DEFAULT 0,
    total_rescued      integer NOT NULL DEFAULT 0,  -- unit terjual dari order selesai
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX merchants_location_idx ON merchants USING gist (location);
CREATE TRIGGER merchants_updated_at BEFORE UPDATE ON merchants FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE merchant_photos (
    merchant_id uuid NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    media_id    uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    position    smallint NOT NULL CHECK (position BETWEEN 0 AND 9),
    PRIMARY KEY (merchant_id, media_id),
    UNIQUE (merchant_id, position)
);

CREATE TABLE addresses (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label        text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 30),
    full_address text NOT NULL,
    location     geography(Point, 4326) NOT NULL,
    note         text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX addresses_user_idx ON addresses (user_id);
ALTER TABLE users ADD CONSTRAINT users_selected_address_fk FOREIGN KEY (selected_address_id) REFERENCES addresses(id) ON DELETE SET NULL;

CREATE TABLE saved_merchants (
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    merchant_id uuid NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, merchant_id)
);
