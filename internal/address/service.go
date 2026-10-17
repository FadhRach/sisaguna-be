package address

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxPerUser = 20

var (
	ErrNotFound     = errors.New("alamat tidak ditemukan")
	ErrLimit        = errors.New("jumlah alamat maksimal 20")
	ErrPartialPoint = errors.New("lat dan lng harus dikirim berpasangan")
)

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]AddressDTO, error) {
	rows, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("address service list: %w", err)
	}
	out := make([]AddressDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto(r.ID, r.Label, r.FullAddress, r.Note, r.Lat, r.Lng))
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateRequest) (*AddressDTO, error) {
	n, err := s.repo.Count(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("address service create: %w", err)
	}
	if n >= maxPerUser {
		return nil, ErrLimit
	}
	r, err := s.repo.Create(ctx, userID, in, strings.TrimSpace(in.Label), strings.TrimSpace(in.FullAddress))
	if err != nil {
		return nil, fmt.Errorf("address service create: %w", err)
	}
	d := dto(r.ID, r.Label, r.FullAddress, r.Note, r.Lat, r.Lng)
	return &d, nil
}

func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, in UpdateRequest) (*AddressDTO, error) {
	if (in.Lat == nil) != (in.Lng == nil) {
		return nil, ErrPartialPoint
	}
	r, err := s.repo.Update(ctx, id, userID, in)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("address service update: %w", err)
	}
	d := dto(r.ID, r.Label, r.FullAddress, r.Note, r.Lat, r.Lng)
	return &d, nil
}

// Delete: kalau alamat yang dihapus sedang terpilih, FK ON DELETE SET NULL mengembalikan ke lokasi GPS.
func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	n, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("address service delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Select memilih alamat aktif; nil berarti lokasi GPS.
func (s *Service) Select(ctx context.Context, userID uuid.UUID, id *uuid.UUID) error {
	sel := uuid.NullUUID{}
	if id != nil {
		ok, err := s.repo.BelongsTo(ctx, *id, userID)
		if err != nil {
			return fmt.Errorf("address service select: %w", err)
		}
		if !ok {
			return ErrNotFound
		}
		sel = uuid.NullUUID{UUID: *id, Valid: true}
	}
	if err := s.repo.Select(ctx, userID, sel); err != nil {
		return fmt.Errorf("address service select: %w", err)
	}
	return nil
}

func dto(id uuid.UUID, label, full, note string, lat, lng float64) AddressDTO {
	return AddressDTO{ID: id.String(), Label: label, FullAddress: full, Note: note, Lat: lat, Lng: lng}
}
