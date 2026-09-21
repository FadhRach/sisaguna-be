package service

import (
	"context"
	"fmt"

	"github.com/FadhRach/sisaguna-be/internal/model"
	"github.com/FadhRach/sisaguna-be/internal/repository"
	"github.com/google/uuid"
)

// UpdateProfileInput pakai pointer supaya field yang tidak dikirim di PATCH
// tidak ikut ditimpa (nil = tidak diubah).
type UpdateProfileInput struct {
	Name         *string
	Phone        *string
	BusinessName *string
}

type UserService interface {
	GetProfile(ctx context.Context, userID uuid.UUID) (*model.User, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, in UpdateProfileInput) (*model.User, error)
	GetPublicProfile(ctx context.Context, userID uuid.UUID) (*model.PublicUser, error)
}

type userService struct {
	userRepo repository.UserRepository
}

func NewUserService(userRepo repository.UserRepository) UserService {
	return &userService{userRepo: userRepo}
}

func (s *userService) GetProfile(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user service get profile: %w", err)
	}
	return user, nil
}

func (s *userService) UpdateProfile(ctx context.Context, userID uuid.UUID, in UpdateProfileInput) (*model.User, error) {
	fields := map[string]interface{}{}
	if in.Name != nil {
		fields["name"] = *in.Name
	}
	if in.Phone != nil {
		fields["phone"] = *in.Phone
	}
	if in.BusinessName != nil {
		fields["business_name"] = *in.BusinessName
	}

	if len(fields) == 0 {
		return s.GetProfile(ctx, userID)
	}

	user, err := s.userRepo.Update(ctx, userID, fields)
	if err != nil {
		return nil, fmt.Errorf("user service update profile: %w", err)
	}
	return user, nil
}

func (s *userService) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*model.PublicUser, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user service get public profile: %w", err)
	}
	return user.ToPublicUser(), nil
}
