CREATE TABLE reviews (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id  uuid NOT NULL REFERENCES reservations(id) ON DELETE CASCADE,
    reviewer_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reviewee_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    direction       text NOT NULL,
    rating          smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment         text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (reservation_id, direction)
);
