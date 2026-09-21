CREATE TABLE impact_records (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id  uuid NOT NULL UNIQUE REFERENCES reservations(id) ON DELETE CASCADE,
    weight_kg       numeric NOT NULL,
    co2_avoided_kg  numeric,
    verified_at     timestamptz
);
