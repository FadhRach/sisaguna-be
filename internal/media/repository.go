package media

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
)

type Repository struct{ q *sqlc.Queries }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{q: sqlc.New(pool)} }

func (r *Repository) Upsert(ctx context.Context, owner uuid.UUID, vis string, size, w, h int, sha, path string, thumb *string) (sqlc.Medium, error) {
	t := pgtype.Text{}
	if thumb != nil {
		t = pgtype.Text{String: *thumb, Valid: true}
	}
	return r.q.UpsertMedia(ctx, sqlc.UpsertMediaParams{
		OwnerID: owner, Visibility: sqlc.MediaVisibility(vis), SizeBytes: int32(size),
		Width: pgtype.Int4{Int32: int32(w), Valid: true}, Height: pgtype.Int4{Int32: int32(h), Valid: true},
		Sha256: sha, Path: path, ThumbPath: t,
	})
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (sqlc.Medium, error) {
	return r.q.GetMedia(ctx, id)
}

func (r *Repository) UpsertVideo(ctx context.Context, owner uuid.UUID, size, durationMs int, sha, path string) (sqlc.Medium, error) {
	return r.q.UpsertVideo(ctx, sqlc.UpsertVideoParams{OwnerID: owner, SizeBytes: int32(size),
		DurationMs: pgtype.Int4{Int32: int32(durationMs), Valid: true}, Sha256: sha, Path: path})
}
