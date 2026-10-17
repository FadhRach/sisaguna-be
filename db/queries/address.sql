-- name: ListAddresses :many
SELECT id, label, full_address, note,
       ST_Y(location::geometry)::float8 AS lat, ST_X(location::geometry)::float8 AS lng
FROM addresses WHERE user_id = $1 ORDER BY created_at, id;

-- name: CountAddresses :one
SELECT count(*) FROM addresses WHERE user_id = $1;

-- name: CreateAddress :one
INSERT INTO addresses (user_id, label, full_address, note, location)
VALUES ($1, $2, $3, $4, ST_SetSRID(ST_MakePoint(@lng::float8, @lat::float8), 4326)::geography)
RETURNING id, label, full_address, note,
          ST_Y(location::geometry)::float8 AS lat, ST_X(location::geometry)::float8 AS lng;

-- name: UpdateAddress :one
UPDATE addresses SET
    label        = coalesce(sqlc.narg('label'), label),
    full_address = coalesce(sqlc.narg('full_address'), full_address),
    note         = coalesce(sqlc.narg('note'), note),
    location     = CASE WHEN sqlc.narg('lat')::float8 IS NOT NULL AND sqlc.narg('lng')::float8 IS NOT NULL
                        THEN ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)::geography
                        ELSE location END
WHERE id = $1 AND user_id = $2
RETURNING id, label, full_address, note,
          ST_Y(location::geometry)::float8 AS lat, ST_X(location::geometry)::float8 AS lng;

-- name: DeleteAddress :execrows
DELETE FROM addresses WHERE id = $1 AND user_id = $2;

-- name: AddressBelongsToUser :one
SELECT EXISTS (SELECT 1 FROM addresses WHERE id = $1 AND user_id = $2);
