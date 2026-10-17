package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/rules"
)

type Profile struct {
	sqlc.GetProfileRow
	UnreadNotifications int64
}

func (s *Service) Me(ctx context.Context, id uuid.UUID) (*Profile, error) {
	p, err := s.repo.Profile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user service me: %w", err)
	}
	unread, err := s.repo.UnreadNotifications(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("user service me: %w", err)
	}
	return &Profile{GetProfileRow: p, UnreadNotifications: unread}, nil
}

// Impact: ringkasan dampak dari order selesai ditambah baseline pengguna. Karbon = estimasi, lihat rules.CarbonKg.
func (s *Service) Impact(ctx context.Context, id uuid.UUID) (*ImpactDTO, error) {
	r, err := s.repo.Impact(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("user service impact: %w", err)
	}
	return &ImpactDTO{CompletedOrders: r.CompletedOrders, Portions: r.Portions, FeedCompostKg: r.FeedCompostKg, SavedRupiah: r.SavedRupiah,
		CarbonKg: int64(rules.CarbonKg(int(r.Portions))), Tier: rules.TierOf(int(r.Portions)).Key}, nil
}

// UpdateProfile mengubah profil secara parsial. photo_media_id harus foto publik milik akun ini;
// foto lama dilepas (attached_at = NULL) supaya dibersihkan scheduler.
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, in UpdateProfileRequest) error {
	err := s.repo.InTx(ctx, func(r *Repository) error {
		cur, err := r.UserByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}

		p := sqlc.UpdateUserProfileParams{ID: id, Phone: text(in.Phone), AvatarKey: text(in.AvatarKey)}
		if in.Email != nil {
			p.Email = pgtype.Text{String: normalizeEmail(*in.Email), Valid: true}
		}
		if in.Name != nil {
			p.Name = pgtype.Text{String: strings.TrimSpace(*in.Name), Valid: true}
		}
		if in.PhotoMediaID.Set {
			p.SetPhoto = true
			if in.PhotoMediaID.Valid {
				m, err := r.Media(ctx, in.PhotoMediaID.Value)
				if errors.Is(err, pgx.ErrNoRows) || (err == nil && (m.OwnerID != id || m.Visibility != sqlc.MediaVisibilityPublic)) {
					return ErrInvalidMedia
				}
				if err != nil {
					return err
				}
				p.PhotoMediaID = uuid.NullUUID{UUID: m.ID, Valid: true}
			}
		}
		if err := r.UpdateProfile(ctx, p); err != nil {
			if database.IsUniqueViolation(err) {
				return ErrEmailTaken
			}
			return err
		}
		if in.PhotoMediaID.Set {
			if cur.PhotoMediaID.Valid && cur.PhotoMediaID != p.PhotoMediaID {
				if err := r.DetachMedia(ctx, cur.PhotoMediaID.UUID); err != nil {
					return err
				}
			}
			if p.PhotoMediaID.Valid {
				return r.AttachMedia(ctx, p.PhotoMediaID.UUID)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrEmailTaken) || errors.Is(err, ErrInvalidMedia) || errors.Is(err, ErrUserNotFound) {
			return err
		}
		return fmt.Errorf("user service update profile: %w", err)
	}
	return nil
}

func text(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
