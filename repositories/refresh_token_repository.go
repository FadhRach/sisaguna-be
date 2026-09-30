package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/FadhRach/sisaguna-be/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RefreshTokenRepository adalah akses data untuk tabel refresh_tokens.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *models.RefreshToken) error
	FindByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID) error
}

type refreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) RefreshTokenRepository {
	return &refreshTokenRepository{db: db}
}

func (r *refreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("refresh token repository create: %w", err)
	}
	return nil
}

func (r *refreshTokenRepository) FindByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	var token models.RefreshToken
	if err := r.db.WithContext(ctx).First(&token, "token_hash = ?", tokenHash).Error; err != nil {
		return nil, fmt.Errorf("refresh token repository find by hash: %w", err)
	}
	return &token, nil
}

func (r *refreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Model(&models.RefreshToken{}).Where("id = ?", id).Update("revoked_at", time.Now()).Error; err != nil {
		return fmt.Errorf("refresh token repository revoke: %w", err)
	}
	return nil
}
