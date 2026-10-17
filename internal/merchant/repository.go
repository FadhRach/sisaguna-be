package merchant

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: sqlc.New(pool)}
}

func (r *Repository) InTx(ctx context.Context, fn func(*Repository) error) error {
	return database.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		return fn(&Repository{pool: r.pool, q: r.q.WithTx(tx)})
	})
}

func (r *Repository) Create(ctx context.Context, p sqlc.CreateMerchantParams) (uuid.UUID, error) {
	return r.q.CreateMerchant(ctx, p)
}

func (r *Repository) ByOwner(ctx context.Context, owner uuid.UUID) (sqlc.GetMerchantByOwnerRow, error) {
	return r.q.GetMerchantByOwner(ctx, owner)
}

func (r *Repository) Approved(ctx context.Context, p sqlc.GetApprovedMerchantParams) (sqlc.GetApprovedMerchantRow, error) {
	return r.q.GetApprovedMerchant(ctx, p)
}

func (r *Repository) IsSaved(ctx context.Context, user, merchant uuid.UUID) (bool, error) {
	return r.q.IsMerchantSaved(ctx, sqlc.IsMerchantSavedParams{UserID: user, MerchantID: merchant})
}

func (r *Repository) Update(ctx context.Context, p sqlc.UpdateMerchantParams) error {
	return r.q.UpdateMerchant(ctx, p)
}

func (r *Repository) SyncListingsLocation(ctx context.Context, id uuid.UUID) error {
	return r.q.SyncListingsLocation(ctx, id)
}

// replaceBanner memasang banner baru dan melepas yang lama (dibersihkan scheduler kalau tidak dipakai lagi).
func (r *Repository) replaceBanner(ctx context.Context, owner, merchant, mediaID uuid.UUID) error {
	old, err := r.q.GetMerchantBanner(ctx, merchant)
	if err != nil {
		return err
	}
	if old.Valid && old.UUID == mediaID {
		return nil
	}
	if err := media.Attach(ctx, r.q, owner, sqlc.MediaKindImage, sqlc.MediaVisibilityPublic, []uuid.UUID{mediaID}, func(uuid.UUID, int16) error { return nil }); err != nil {
		return err
	}
	if old.Valid {
		return r.q.DetachMedia(ctx, old.UUID)
	}
	return nil
}

// replacePhotos mengganti seluruh galeri (maks 10, posisi = urutan).
func (r *Repository) replacePhotos(ctx context.Context, owner, merchant uuid.UUID, ids []uuid.UUID) error {
	olds, err := r.q.DeleteMerchantPhotos(ctx, merchant)
	if err != nil {
		return err
	}
	for _, id := range olds {
		if err := r.q.DetachMedia(ctx, id); err != nil {
			return err
		}
	}
	return media.Attach(ctx, r.q, owner, sqlc.MediaKindImage, sqlc.MediaVisibilityPublic, ids, func(id uuid.UUID, pos int16) error {
		return r.q.InsertMerchantPhoto(ctx, sqlc.InsertMerchantPhotoParams{MerchantID: merchant, MediaID: id, Position: pos})
	})
}
