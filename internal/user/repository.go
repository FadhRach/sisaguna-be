package user

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: sqlc.New(pool)}
}

// InTx menjalankan fn dengan repository yang terikat satu transaksi.
func (r *Repository) InTx(ctx context.Context, fn func(*Repository) error) error {
	return database.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		return fn(&Repository{pool: r.pool, q: r.q.WithTx(tx)})
	})
}

func (r *Repository) CreateUser(ctx context.Context, name, email, phone, hash string) (sqlc.User, error) {
	return r.q.CreateUser(ctx, sqlc.CreateUserParams{Name: name, Email: email, Phone: phone, PasswordHash: hash})
}

func (r *Repository) UserByEmail(ctx context.Context, email string) (sqlc.User, error) {
	return r.q.GetUserByEmail(ctx, email)
}

func (r *Repository) UserByID(ctx context.Context, id uuid.UUID) (sqlc.User, error) {
	return r.q.GetUserByID(ctx, id)
}

func (r *Repository) UnreadNotifications(ctx context.Context, id uuid.UUID) (int64, error) {
	return r.q.CountUnreadNotifications(ctx, id)
}

// Impact mengembalikan angka nol kalau pengguna belum punya order selesai.
func (r *Repository) Impact(ctx context.Context, id uuid.UUID) (sqlc.UserImpactRow, error) {
	row, err := r.q.UserImpact(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.UserImpactRow{}, nil
	}
	return row, err
}

func (r *Repository) CreateRefreshToken(ctx context.Context, userID, family uuid.UUID, hash, userAgent string, expires time.Time) (sqlc.RefreshToken, error) {
	return r.q.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		UserID: userID, FamilyID: family, TokenHash: hash,
		UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})
}

func (r *Repository) RefreshTokenForUpdate(ctx context.Context, hash string) (sqlc.RefreshToken, error) {
	return r.q.GetRefreshTokenByHashForUpdate(ctx, hash)
}

func (r *Repository) RotateRefreshToken(ctx context.Context, oldID, newID uuid.UUID) error {
	return r.q.RotateRefreshToken(ctx, sqlc.RotateRefreshTokenParams{ID: oldID, ReplacedBy: uuid.NullUUID{UUID: newID, Valid: true}})
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return r.q.RevokeRefreshToken(ctx, id)
}

func (r *Repository) RevokeFamily(ctx context.Context, family uuid.UUID) error {
	return r.q.RevokeRefreshTokenFamily(ctx, family)
}

func (r *Repository) Profile(ctx context.Context, id uuid.UUID) (sqlc.GetProfileRow, error) {
	return r.q.GetProfile(ctx, id)
}

func (r *Repository) UpdateProfile(ctx context.Context, p sqlc.UpdateUserProfileParams) error {
	return r.q.UpdateUserProfile(ctx, p)
}

func (r *Repository) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	return r.q.UpdatePasswordHash(ctx, sqlc.UpdatePasswordHashParams{ID: id, PasswordHash: hash})
}

func (r *Repository) RefreshTokenByHash(ctx context.Context, hash string) (sqlc.RefreshToken, error) {
	return r.q.GetRefreshTokenByHash(ctx, hash)
}

func (r *Repository) RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.q.RevokeAllUserRefreshTokens(ctx, userID)
}

func (r *Repository) Media(ctx context.Context, id uuid.UUID) (sqlc.Medium, error) {
	return r.q.GetMedia(ctx, id)
}

func (r *Repository) AttachMedia(ctx context.Context, id uuid.UUID) error {
	return r.q.AttachMedia(ctx, id)
}

func (r *Repository) DetachMedia(ctx context.Context, id uuid.UUID) error {
	return r.q.DetachMedia(ctx, id)
}
