package voucher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/apperr"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
)

var (
	ErrInvalid          = errors.New("voucher tidak valid")
	ErrCodeTaken        = errors.New("kode voucher sudah dipakai")
	ErrNotFound         = errors.New("voucher tidak ditemukan")
	ErrMerchantNotFound = errors.New("toko tidak ditemukan")
)

func invalid(detail string) error {
	return apperr.WithFields(ErrInvalid, map[string]any{"detail": detail})
}

// Create membuat voucher baru. Aturan: tepat satu dari percent_off / flat_off, max_discount hanya untuk persen,
// masa berlaku harus berakhir sesudah mulai.
func (s *Service) Create(ctx context.Context, in AdminCreateRequest) error {
	if (in.PercentOff == nil) == (in.FlatOff == nil) {
		return invalid("isi tepat satu dari percent_off atau flat_off")
	}
	if in.MaxDiscount != nil && in.PercentOff == nil {
		return invalid("max_discount hanya untuk voucher persen")
	}
	if in.PercentOff != nil && (*in.PercentOff < 1 || *in.PercentOff > 100) || in.FlatOff != nil && *in.FlatOff < 1 ||
		in.MaxDiscount != nil && *in.MaxDiscount < 1 || in.Quota != nil && *in.Quota < 1 {
		return invalid("nilai diskon, batas maksimum, dan kuota harus lebih dari 0 (persen maks 100)")
	}
	starts := time.Now()
	if in.StartsAt != nil {
		starts = *in.StartsAt
	}
	if !in.ExpiresAt.After(starts) {
		return invalid("expires_at harus setelah starts_at")
	}
	if in.MerchantID != nil {
		if _, err := s.q.GetMerchantBrief(ctx, *in.MerchantID); errors.Is(err, pgx.ErrNoRows) {
			return ErrMerchantNotFound
		} else if err != nil {
			return fmt.Errorf("voucher service create: %w", err)
		}
	}
	p := sqlc.CreateVoucherParams{Code: in.Code, Title: strings.TrimSpace(in.Title), Description: in.Description, MinSpend: int32(in.MinSpend),
		ExpiresAt: pgtype.Timestamptz{Time: in.ExpiresAt, Valid: true}, Terms: in.Terms, AccentHex: "#55B931"}
	if p.Terms == nil {
		p.Terms = []string{}
	}
	if in.AccentHex != "" {
		p.AccentHex = in.AccentHex
	}
	if in.PercentOff != nil {
		p.PercentOff = pgtype.Int2{Int16: int16(*in.PercentOff), Valid: true}
	}
	if in.FlatOff != nil {
		p.FlatOff = pgtype.Int4{Int32: int32(*in.FlatOff), Valid: true}
	}
	if in.MaxDiscount != nil {
		p.MaxDiscount = pgtype.Int4{Int32: int32(*in.MaxDiscount), Valid: true}
	}
	if in.Quota != nil {
		p.Quota = pgtype.Int4{Int32: int32(*in.Quota), Valid: true}
	}
	if in.MerchantID != nil {
		p.MerchantID = uuid.NullUUID{UUID: *in.MerchantID, Valid: true}
	}
	p.AppliesToLabel = in.AppliesTo
	if in.ImageMediaID != nil {
		p.ImageMediaID = uuid.NullUUID{UUID: *in.ImageMediaID, Valid: true}
	}
	if in.AppliesToTier != "" {
		p.AppliesToTier = sqlc.NullListingTier{ListingTier: sqlc.ListingTier(in.AppliesToTier), Valid: true}
	}
	if in.StartsAt != nil {
		p.StartsAt = pgtype.Timestamptz{Time: *in.StartsAt, Valid: true}
	}
	if err := s.q.CreateVoucher(ctx, p); database.IsUniqueViolation(err) {
		return ErrCodeTaken
	} else if err != nil {
		return fmt.Errorf("voucher service create: %w", err)
	}
	return nil
}

// Update mengubah bagian yang aman setelah voucher beredar: judul, deskripsi, min_spend, masa berlaku, kuota,
// status aktif, syarat, warna. Jenis dan besar diskon sengaja tidak bisa diubah.
func (s *Service) Update(ctx context.Context, code string, in AdminUpdateRequest) error {
	v, err := s.q.GetVoucherState(ctx, strings.ToUpper(code))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("voucher service update: %w", err)
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(v.Voucher.StartsAt.Time) {
		return invalid("expires_at harus setelah starts_at")
	}
	if in.Quota != nil && (*in.Quota < 1 || *in.Quota < int(v.Voucher.UsedCount)) {
		return invalid("kuota tidak boleh di bawah jumlah yang sudah terpakai")
	}
	p := sqlc.UpdateVoucherParams{Code: v.Voucher.Code, Terms: in.Terms}
	if in.Title != nil {
		p.Title = pgtype.Text{String: strings.TrimSpace(*in.Title), Valid: true}
	}
	if in.Description != nil {
		p.Description = pgtype.Text{String: *in.Description, Valid: true}
	}
	if in.MinSpend != nil {
		p.MinSpend = pgtype.Int4{Int32: int32(*in.MinSpend), Valid: true}
	}
	if in.ExpiresAt != nil {
		p.ExpiresAt = pgtype.Timestamptz{Time: *in.ExpiresAt, Valid: true}
	}
	if in.Quota != nil {
		p.Quota = pgtype.Int4{Int32: int32(*in.Quota), Valid: true}
	}
	if in.IsActive != nil {
		p.IsActive = pgtype.Bool{Bool: *in.IsActive, Valid: true}
	}
	if in.AccentHex != nil {
		p.AccentHex = pgtype.Text{String: *in.AccentHex, Valid: true}
	}
	if in.AppliesTo != nil {
		p.AppliesToLabel = pgtype.Text{String: *in.AppliesTo, Valid: true}
	}
	if in.ImageMediaID != nil {
		p.ImageMediaID = uuid.NullUUID{UUID: *in.ImageMediaID, Valid: true}
	}
	if err := s.q.UpdateVoucher(ctx, p); err != nil {
		return fmt.Errorf("voucher service update: %w", err)
	}
	return nil
}

func (s *Service) AdminList(ctx context.Context, page, limit int) ([]AdminVoucherDTO, int64, error) {
	rows, err := s.q.ListVouchersAdmin(ctx, sqlc.ListVouchersAdminParams{Lim: int32(limit), Off: int32((page - 1) * limit)})
	if err != nil {
		return nil, 0, fmt.Errorf("voucher service admin list: %w", err)
	}
	var total int64
	out := make([]AdminVoucherDTO, 0, len(rows))
	for _, r := range rows {
		total = r.Total
		out = append(out, adminDTO(r.Voucher))
	}
	return out, total, nil
}

func adminDTO(v sqlc.Voucher) AdminVoucherDTO {
	d := AdminVoucherDTO{Code: v.Code, Title: v.Title, Description: v.Description, PercentOff: int2Ptr(v.PercentOff), FlatOff: int4Ptr(v.FlatOff),
		MaxDiscount: int4Ptr(v.MaxDiscount), MinSpend: int(v.MinSpend), StartsAt: v.StartsAt.Time.UTC(), ExpiresAt: v.ExpiresAt.Time.UTC(),
		Quota: int4Ptr(v.Quota), UsedCount: int(v.UsedCount), Terms: v.Terms, AccentHex: v.AccentHex, IsActive: v.IsActive, CreatedAt: v.CreatedAt.Time.UTC()}
	if v.MerchantID.Valid {
		m := v.MerchantID.UUID.String()
		d.MerchantID = &m
	}
	if v.AppliesToTier.Valid {
		t := string(v.AppliesToTier.ListingTier)
		d.AppliesToTier = &t
	}
	d.AppliesTo = v.AppliesToLabel
	if v.ImageMediaID.Valid {
		id := v.ImageMediaID.UUID.String()
		d.ImageMediaID = &id
	}
	if d.Terms == nil {
		d.Terms = []string{}
	}
	return d
}

// Get dipakai handler untuk mengembalikan voucher yang baru dibuat/diubah.
func (s *Service) Get(ctx context.Context, code string) (*AdminVoucherDTO, error) {
	v, err := s.q.GetVoucherState(ctx, strings.ToUpper(code))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("voucher service get: %w", err)
	}
	d := adminDTO(v.Voucher)
	return &d, nil
}
