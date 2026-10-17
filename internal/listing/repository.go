package listing

import (
	"context"

	"github.com/jackc/pgx/v5"
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

func (r *Repository) InTx(ctx context.Context, fn func(*Repository) error) error {
	return database.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		return fn(&Repository{pool: r.pool, q: r.q.WithTx(tx)})
	})
}
