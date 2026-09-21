CREATE TABLE reservations (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_id         uuid NOT NULL REFERENCES listings(id) ON DELETE CASCADE,
    penerima_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pickup_slot_start  timestamptz NOT NULL,
    pickup_slot_end    timestamptz NOT NULL,
    status             text NOT NULL DEFAULT 'pending',
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_reservations_listing_id ON reservations(listing_id);
CREATE INDEX idx_reservations_penerima_id ON reservations(penerima_id);
