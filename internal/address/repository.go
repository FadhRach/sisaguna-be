package address

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
)

type Repository struct{ q *sqlc.Queries }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{q: sqlc.New(pool)} }

func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]sqlc.ListAddressesRow, error) {
	return r.q.ListAddresses(ctx, userID)
}

func (r *Repository) Count(ctx context.Context, userID uuid.UUID) (int64, error) {
	return r.q.CountAddresses(ctx, userID)
}

func (r *Repository) Create(ctx context.Context, userID uuid.UUID, in CreateRequest, label, full string) (sqlc.CreateAddressRow, error) {
	return r.q.CreateAddress(ctx, sqlc.CreateAddressParams{
		UserID: userID, Label: label, FullAddress: full, Note: in.Note, Lat: *in.Lat, Lng: *in.Lng,
	})
}

func (r *Repository) Update(ctx context.Context, id, userID uuid.UUID, in UpdateRequest) (sqlc.UpdateAddressRow, error) {
	return r.q.UpdateAddress(ctx, sqlc.UpdateAddressParams{
		ID: id, UserID: userID,
		Label: text(in.Label), FullAddress: text(in.FullAddress), Note: text(in.Note),
		Lat: float(in.Lat), Lng: float(in.Lng),
	})
}

func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) (int64, error) {
	return r.q.DeleteAddress(ctx, sqlc.DeleteAddressParams{ID: id, UserID: userID})
}

func (r *Repository) BelongsTo(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	return r.q.AddressBelongsToUser(ctx, sqlc.AddressBelongsToUserParams{ID: id, UserID: userID})
}

func (r *Repository) Select(ctx context.Context, userID uuid.UUID, id uuid.NullUUID) error {
	return r.q.SetSelectedAddress(ctx, sqlc.SetSelectedAddressParams{ID: userID, SelectedAddressID: id})
}

func text(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func float(f *float64) pgtype.Float8 {
	if f == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *f, Valid: true}
}
