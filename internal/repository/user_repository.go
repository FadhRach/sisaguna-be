// Package repository adalah lapisan akses data: query GORM per resource,
// tidak ada business logic di sini (itu tanggung jawab package service).
package repository

import (
	"context"
	"fmt"

	"github.com/FadhRach/sisaguna-be/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserRepository adalah akses data untuk tabel users.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	Update(ctx context.Context, id uuid.UUID, fields map[string]interface{}) (*model.User, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("user repository create: %w", err)
	}
	return nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("user repository find by id: %w", err)
	}
	return &user, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return nil, fmt.Errorf("user repository find by email: %w", err)
	}
	return &user, nil
}

// Update pakai map (bukan Updates(struct)) supaya field yang memang di-set
// kosong tetap tersimpan — GORM Updates(struct) diam-diam skip zero value.
func (r *userRepository) Update(ctx context.Context, id uuid.UUID, fields map[string]interface{}) (*model.User, error) {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return nil, fmt.Errorf("user repository update: %w", err)
	}
	return r.FindByID(ctx, id)
}
