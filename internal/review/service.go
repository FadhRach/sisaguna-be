// Package review: ulasan toko dari pembeli setelah order selesai.
package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/notification"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

const window = 7 * 24 * time.Hour

var (
	ErrOrderNotFound   = errors.New("pesanan tidak ditemukan")
	ErrNotCompleted    = errors.New("ulasan hanya untuk pesanan yang sudah selesai")
	ErrWindowClosed    = errors.New("batas 7 hari untuk memberi ulasan sudah lewat")
	ErrAlreadyReviewed = errors.New("pesanan ini sudah diulas")
	ErrMerchantMissing = errors.New("toko tidak ditemukan")
)

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	urls pmedia.URLs
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, urls pmedia.URLs) *Service {
	return &Service{pool: pool, q: sqlc.New(pool), urls: urls, now: time.Now}
}

// Create: hanya pembeli order yang `completed`, sekali per order, maksimal 7 hari setelah selesai.
// Rating toko dihitung ulang di transaksi yang sama.
func (s *Service) Create(ctx context.Context, buyer, orderID uuid.UUID, in CreateRequest) (*ReviewDTO, error) {
	var out ReviewDTO
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		o, err := q.LockOrder(ctx, orderID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && o.BuyerID != buyer) {
			return ErrOrderNotFound
		}
		if err != nil {
			return err
		}
		if o.Status != sqlc.OrderStatusCompleted {
			return ErrNotCompleted
		}
		if s.now().Sub(o.CompletedAt.Time) > window {
			return ErrWindowClosed
		}
		tags := make([]string, 0, len(in.Tags))
		for _, t := range in.Tags {
			tags = append(tags, strings.TrimSpace(t))
		}
		row, err := q.InsertReview(ctx, sqlc.InsertReviewParams{OrderID: o.ID, MerchantID: o.MerchantID, BuyerID: buyer,
			Stars: int16(in.Stars), Tags: tags, Comment: strings.TrimSpace(in.Comment)})
		if database.IsUniqueViolation(err) {
			return ErrAlreadyReviewed
		}
		if err != nil {
			return err
		}
		err = media.Attach(ctx, q, buyer, sqlc.MediaKindImage, sqlc.MediaVisibilityPublic, in.MediaIDs, func(id uuid.UUID, pos int16) error {
			return q.AttachReviewMedia(ctx, sqlc.AttachReviewMediaParams{ReviewID: row.ID, MediaID: id, Position: pos})
		})
		if err != nil {
			return err
		}
		if err := q.RecalcMerchantRating(ctx, o.MerchantID); err != nil {
			return err
		}
		m, err := q.GetMerchantBrief(ctx, o.MerchantID)
		if err != nil {
			return err
		}
		if err := notification.Create(ctx, q, notification.New{UserID: m.OwnerID, Type: sqlc.NotificationTypeMerchant,
			Title: "Ulasan baru", Body: fmt.Sprintf("Pembeli memberi %d bintang untuk %s.", in.Stars, m.Name),
			Data: map[string]any{"order_id": o.ID}}); err != nil {
			return err
		}
		out = ReviewDTO{ID: row.ID.String(), Stars: in.Stars, Tags: tags, Comment: strings.TrimSpace(in.Comment),
			Photos: []PhotoDTO{}, CreatedAt: row.CreatedAt.Time.UTC()}
		return nil
	})
	if err != nil {
		return nil, wrap("review service create", err)
	}
	return &out, nil
}

// ListByMerchant: ulasan toko approved, terbaru dulu, cursor.
func (s *Service) ListByMerchant(ctx context.Context, merchantID uuid.UUID, cur *httpx.Cursor, limit int) ([]ReviewDTO, string, error) {
	m, err := s.q.GetMerchantBrief(ctx, merchantID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.Status != sqlc.MerchantStatusApproved) {
		return nil, "", ErrMerchantMissing
	}
	if err != nil {
		return nil, "", fmt.Errorf("review service list: %w", err)
	}
	p := sqlc.ListReviewsParams{MerchantID: merchantID, Lim: int32(limit + 1)}
	if cur != nil {
		p.CurAt = pgtype.Timestamptz{Time: cur.At, Valid: true}
		p.CurID = uuid.NullUUID{UUID: cur.ID, Valid: true}
	}
	rows, err := s.q.ListReviews(ctx, p)
	if err != nil {
		return nil, "", fmt.Errorf("review service list: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = httpx.EncodeCursor(last.CreatedAt.Time, last.ID)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	photos, err := s.q.ListReviewMedia(ctx, ids)
	if err != nil {
		return nil, "", fmt.Errorf("review service list: %w", err)
	}
	byReview := map[uuid.UUID][]PhotoDTO{}
	for _, p := range photos {
		d := PhotoDTO{URL: s.urls.Public(p.Path)}
		if p.ThumbPath.Valid {
			t := s.urls.Public(p.ThumbPath.String)
			d.ThumbURL = &t
		}
		byReview[p.ReviewID] = append(byReview[p.ReviewID], d)
	}
	out := make([]ReviewDTO, 0, len(rows))
	for _, r := range rows {
		d := ReviewDTO{ID: r.ID.String(), Stars: int(r.Stars), Tags: r.Tags, Comment: r.Comment,
			BuyerName: displayName(r.BuyerName), Photos: byReview[r.ID], CreatedAt: r.CreatedAt.Time.UTC()}
		if d.Photos == nil {
			d.Photos = []PhotoDTO{}
		}
		if d.Tags == nil {
			d.Tags = []string{}
		}
		out = append(out, d)
	}
	return out, next, nil
}

// displayName: ulasan bersifat publik, jadi nama belakang dipendekkan menjadi inisial ("Budi Santoso" -> "Budi S.").
// Nama satu kata, nama belakang yang sudah inisial ("Dinda P.") atau angka ("RW 04") dipakai apa adanya.
func displayName(full string) string {
	f := strings.Fields(full)
	switch {
	case len(f) == 0:
		return "Pembeli"
	case len(f) == 1 || strings.HasSuffix(f[len(f)-1], ".") || unicode.IsDigit([]rune(f[len(f)-1])[0]):
		return strings.Join(f, " ")
	}
	return strings.Join(f[:len(f)-1], " ") + " " + string([]rune(f[len(f)-1])[:1]) + "."
}

// wrap meneruskan error yang dikenal client apa adanya dan membungkus sisanya dengan konteks.
func wrap(ctx string, err error) error {
	for _, known := range []error{ErrOrderNotFound, ErrNotCompleted, ErrWindowClosed, ErrAlreadyReviewed, media.ErrInvalidMedia} {
		if errors.Is(err, known) {
			return err
		}
	}
	return fmt.Errorf("%s: %w", ctx, err)
}
