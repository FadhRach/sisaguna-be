// Package service berisi business logic per resource — validasi, orkestrasi
// antar repository, dipanggil dari package handler. Tidak tahu apa-apa soal
// HTTP (tidak import net/http).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FadhRach/sisaguna-be/internal/model"
	"github.com/FadhRach/sisaguna-be/internal/model/types"
	"github.com/FadhRach/sisaguna-be/internal/repository"
	"github.com/FadhRach/sisaguna-be/internal/util"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrEmailTaken          = errors.New("email sudah terdaftar")
	ErrInvalidRole         = errors.New("role tidak valid")
	ErrInvalidCredentials  = errors.New("email atau password salah")
	ErrInvalidRefreshToken = errors.New("refresh token tidak valid atau kedaluwarsa")
)

type RegisterInput struct {
	Name         string
	Email        string
	Phone        string
	Password     string
	Roles        []string
	BusinessName *string
}

type LoginInput struct {
	Email    string
	Password string
}

// LoginResult berisi sepasang token: AccessToken (JWT, pendek, dipakai tiap
// request) dan RefreshToken (opaque, panjang, dipakai cuma buat POST /auth/refresh).
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	User         *model.User
}

type RefreshResult struct {
	AccessToken string
	ExpiresAt   time.Time
}

// AuthService menangani register, login, refresh access token, dan logout
// (revoke refresh token). Ini masih versi sederhana: satu refresh token tidak
// dirotasi tiap dipakai, dan belum ada manajemen multi-device — itu baru
// masuk "refresh token flow lengkap" di Fase 2 (lihat CLAUDE.md).
type AuthService interface {
	Register(ctx context.Context, in RegisterInput) (*model.User, error)
	Login(ctx context.Context, in LoginInput) (*LoginResult, error)
	Refresh(ctx context.Context, refreshToken string) (*RefreshResult, error)
	Logout(ctx context.Context, userID uuid.UUID, refreshToken string) error
}

type authService struct {
	userRepo         repository.UserRepository
	refreshTokenRepo repository.RefreshTokenRepository
	jwtSecret        string
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

func NewAuthService(
	userRepo repository.UserRepository,
	refreshTokenRepo repository.RefreshTokenRepository,
	jwtSecret string,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) AuthService {
	return &authService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		jwtSecret:        jwtSecret,
		accessTokenTTL:   accessTokenTTL,
		refreshTokenTTL:  refreshTokenTTL,
	}
}

func (s *authService) Register(ctx context.Context, in RegisterInput) (*model.User, error) {
	if err := validateRoles(in.Roles); err != nil {
		return nil, err
	}

	_, err := s.userRepo.FindByEmail(ctx, in.Email)
	if err == nil {
		return nil, ErrEmailTaken
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("auth service register: %w", err)
	}

	passwordHash, err := util.HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("auth service register: %w", err)
	}

	user := &model.User{
		Name:         in.Name,
		Email:        in.Email,
		Phone:        in.Phone,
		PasswordHash: passwordHash,
		Roles:        types.StringArray(in.Roles),
		BusinessName: in.BusinessName,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("auth service register: %w", err)
	}

	return user, nil
}

func (s *authService) Login(ctx context.Context, in LoginInput) (*LoginResult, error) {
	user, err := s.userRepo.FindByEmail(ctx, in.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("auth service login: %w", err)
	}

	// Pesan error generik biar tidak bocorkan email terdaftar atau tidak.
	if err := util.ComparePassword(user.PasswordHash, in.Password); err != nil {
		return nil, ErrInvalidCredentials
	}

	accessToken, expiresAt, err := util.GenerateToken(s.jwtSecret, s.accessTokenTTL, user.ID, user.Email, user.Roles)
	if err != nil {
		return nil, fmt.Errorf("auth service login: %w", err)
	}

	refreshToken, err := s.issueRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("auth service login: %w", err)
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		User:         user,
	}, nil
}

func (s *authService) Refresh(ctx context.Context, refreshToken string) (*RefreshResult, error) {
	stored, err := s.findUsableRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth service refresh: %w", err)
	}

	accessToken, expiresAt, err := util.GenerateToken(s.jwtSecret, s.accessTokenTTL, user.ID, user.Email, user.Roles)
	if err != nil {
		return nil, fmt.Errorf("auth service refresh: %w", err)
	}

	return &RefreshResult{AccessToken: accessToken, ExpiresAt: expiresAt}, nil
}

func (s *authService) Logout(ctx context.Context, userID uuid.UUID, refreshToken string) error {
	stored, err := s.findUsableRefreshToken(ctx, refreshToken)
	if err != nil {
		return err
	}

	// Pastikan refresh token yang mau di-revoke memang milik user yang login,
	// bukan milik orang lain.
	if stored.UserID != userID {
		return ErrInvalidRefreshToken
	}

	if err := s.refreshTokenRepo.Revoke(ctx, stored.ID); err != nil {
		return fmt.Errorf("auth service logout: %w", err)
	}
	return nil
}

// issueRefreshToken membuat refresh token baru dan menyimpan hash-nya, dipanggil
// tiap Login berhasil.
func (s *authService) issueRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	refreshToken, err := util.GenerateOpaqueToken()
	if err != nil {
		return "", err
	}

	record := &model.RefreshToken{
		UserID:    userID,
		TokenHash: util.HashToken(refreshToken),
		ExpiresAt: time.Now().Add(s.refreshTokenTTL),
	}
	if err := s.refreshTokenRepo.Create(ctx, record); err != nil {
		return "", err
	}

	return refreshToken, nil
}

// findUsableRefreshToken mencari refresh token dari nilai mentahnya dan
// memastikan belum kedaluwarsa/direvoke — dipakai bareng oleh Refresh dan Logout.
func (s *authService) findUsableRefreshToken(ctx context.Context, refreshToken string) (*model.RefreshToken, error) {
	stored, err := s.refreshTokenRepo.FindByHash(ctx, util.HashToken(refreshToken))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	if !stored.IsUsable() {
		return nil, ErrInvalidRefreshToken
	}
	return stored, nil
}

func validateRoles(roles []string) error {
	if len(roles) == 0 {
		return ErrInvalidRole
	}
	for _, role := range roles {
		if !isValidRole(role) {
			return ErrInvalidRole
		}
	}
	return nil
}

func isValidRole(role string) bool {
	for _, valid := range model.ValidRoles {
		if role == valid {
			return true
		}
	}
	return false
}
