package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
)

type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	User         sqlc.GetProfileRow
	refreshID    uuid.UUID
}

// Register membuat akun lalu langsung membuka sesi. account_type hanya dipakai validasi;
// toko dibuat lewat POST /merchants (M2).
func (s *Service) Register(ctx context.Context, in RegisterRequest, userAgent string) (*Session, error) {
	hash, err := s.passwords.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("user service register: %w", err)
	}
	u, err := s.repo.CreateUser(ctx, strings.TrimSpace(in.Name), normalizeEmail(in.Email), in.Phone, hash)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("user service register: %w", err)
	}
	sess, err := s.openSession(ctx, s.repo, u, uuid.New(), userAgent)
	if err != nil {
		return nil, fmt.Errorf("user service register: %w", err)
	}
	return sess, nil
}

func (s *Service) Login(ctx context.Context, in LoginRequest, userAgent string) (*Session, error) {
	u, err := s.repo.UserByEmail(ctx, normalizeEmail(in.Email))
	if errors.Is(err, pgx.ErrNoRows) {
		s.passwords.VerifyDummy(in.Password) // samakan waktu respons dengan email yang ada
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("user service login: %w", err)
	}
	if !s.passwords.Verify(u.PasswordHash, in.Password) {
		return nil, ErrInvalidCredentials
	}
	sess, err := s.openSession(ctx, s.repo, u, uuid.New(), userAgent)
	if err != nil {
		return nil, fmt.Errorf("user service login: %w", err)
	}
	return sess, nil
}

// Refresh merotasi refresh token. Token yang sudah dirotasi/dicabut lalu dipakai lagi
// dianggap dicuri: seluruh family dicabut.
func (s *Service) Refresh(ctx context.Context, raw, userAgent string) (*Session, error) {
	var sess *Session
	reused := false
	err := s.repo.InTx(ctx, func(r *Repository) error {
		old, err := r.RefreshTokenForUpdate(ctx, auth.HashToken(raw))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidRefreshToken
		}
		if err != nil {
			return err
		}
		if old.RevokedAt.Valid {
			reused = true
			return r.RevokeFamily(ctx, old.FamilyID) // di-commit dulu, error dikembalikan setelah tx
		}
		if !old.ExpiresAt.Time.After(time.Now()) {
			return ErrInvalidRefreshToken
		}
		u, err := r.UserByID(ctx, old.UserID)
		if err != nil {
			return err
		}
		sess, err = s.openSession(ctx, r, u, old.FamilyID, userAgent)
		if err != nil {
			return err
		}
		return r.RotateRefreshToken(ctx, old.ID, sess.refreshID)
	})
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			return nil, err
		}
		return nil, fmt.Errorf("user service refresh: %w", err)
	}
	if reused {
		return nil, ErrInvalidRefreshToken
	}
	return sess, nil
}

func (s *Service) Logout(ctx context.Context, userID uuid.UUID, raw string) error {
	rec, err := s.repo.RefreshTokenByHash(ctx, auth.HashToken(raw))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && rec.UserID != userID) {
		return ErrInvalidRefreshToken
	}
	if err != nil {
		return fmt.Errorf("user service logout: %w", err)
	}
	if err := s.repo.RevokeRefreshToken(ctx, rec.ID); err != nil {
		return fmt.Errorf("user service logout: %w", err)
	}
	return nil
}

// ChangePassword mengganti password, mencabut semua refresh token (semua perangkat),
// lalu membuka sesi baru supaya perangkat ini tetap login.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, current, next, userAgent string) (*Session, error) {
	hash, err := s.passwords.Hash(next)
	if err != nil {
		return nil, fmt.Errorf("user service change password: %w", err)
	}
	var sess *Session
	err = s.repo.InTx(ctx, func(r *Repository) error {
		u, err := r.UserByID(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		if !s.passwords.Verify(u.PasswordHash, current) {
			return ErrWrongPassword
		}
		if err := r.UpdatePasswordHash(ctx, userID, hash); err != nil {
			return err
		}
		if err := r.RevokeAllRefreshTokens(ctx, userID); err != nil {
			return err
		}
		sess, err = s.openSession(ctx, r, u, uuid.New(), userAgent)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrWrongPassword) || errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("user service change password: %w", err)
	}
	return sess, nil
}

// openSession menerbitkan access token + refresh token baru dalam family yang diberikan.
func (s *Service) openSession(ctx context.Context, r *Repository, u sqlc.User, family uuid.UUID, userAgent string) (*Session, error) {
	access, exp, err := s.tokens.Issue(u.ID, u.IsAdmin)
	if err != nil {
		return nil, err
	}
	refresh, err := auth.NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	rec, err := r.CreateRefreshToken(ctx, u.ID, family, auth.HashToken(refresh), userAgent, time.Now().Add(s.refreshTTL))
	if err != nil {
		return nil, err
	}
	profile, err := r.Profile(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &Session{AccessToken: access, RefreshToken: refresh, ExpiresAt: exp, User: profile, refreshID: rec.ID}, nil
}
