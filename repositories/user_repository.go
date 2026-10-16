// Package repositories adalah lapisan akses data: query GORM per resource,
// tidak ada business logic di sini (itu tanggung jawab package services).
package repositories

import (
	"context"
	"fmt"

	"github.com/FadhRach/sisaguna-be/config"
	"github.com/FadhRach/sisaguna-be/models"
	"github.com/google/uuid"
)

// UserRepository adalah akses data untuk tabel users.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, id uuid.UUID, fields map[string]interface{}) (*models.User, error)
}

type userRepository struct{}

func NewUserRepository() UserRepository {
	return &userRepository{}
}

func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	if err := config.DB.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("user repository create: %w", err)
	}
	return nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	if err := config.DB.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("user repository find by id: %w", err)
	}
	return &user, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := config.DB.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return nil, fmt.Errorf("user repository find by email: %w", err)
	}
	return &user, nil
}

// Update pakai map (bukan Updates(struct)) supaya field yang memang di-set
// kosong tetap tersimpan — GORM Updates(struct) diam-diam skip zero value.
func (r *userRepository) Update(ctx context.Context, id uuid.UUID, fields map[string]interface{}) (*models.User, error) {
	if err := config.DB.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return nil, fmt.Errorf("user repository update: %w", err)
	}
	return r.FindByID(ctx, id)
}
