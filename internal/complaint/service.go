// Package complaint: komplain pembeli atas order selesai, dan penanganannya oleh admin.
// Fase 1 tunai: tidak ada refund otomatis (refund_amount tetap 0).
package complaint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/notification"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/platform/dbconv"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

const window = 24 * time.Hour

var (
	ErrOrderNotFound    = errors.New("pesanan tidak ditemukan")
	ErrNotCompleted     = errors.New("komplain hanya untuk pesanan yang sudah selesai")
	ErrWindowClosed     = errors.New("batas 24 jam untuk komplain sudah lewat")
	ErrAlreadyComplaint = errors.New("pesanan ini sudah dikomplain")
	ErrNotFound         = errors.New("komplain tidak ditemukan")
	ErrClosed           = errors.New("komplain ini sudah ditutup")
	ErrNoteRequired     = errors.New("catatan penyelesaian wajib diisi")
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

// Create: hanya pembeli order `completed`, maksimal 24 jam setelah selesai, sekali per order, foto privat maks 3.
func (s *Service) Create(ctx context.Context, buyer, orderID uuid.UUID, in CreateRequest) (*ComplaintDTO, error) {
	var out ComplaintDTO
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
		detail := strings.TrimSpace(in.Detail)
		row, err := q.InsertComplaint(ctx, sqlc.InsertComplaintParams{OrderID: o.ID, BuyerID: buyer, MerchantID: o.MerchantID,
			Reason: sqlc.ComplaintReason(in.Reason), Detail: detail})
		if database.IsUniqueViolation(err) {
			return ErrAlreadyComplaint
		}
		if err != nil {
			return err
		}
		urls := []string{}
		err = media.Attach(ctx, q, buyer, sqlc.MediaKindImage, sqlc.MediaVisibilityPrivate, in.MediaIDs, func(id uuid.UUID, pos int16) error {
			urls = append(urls, s.urls.Private(id.String()))
			return q.AttachComplaintMedia(ctx, sqlc.AttachComplaintMediaParams{ComplaintID: row.ID, MediaID: id, Position: pos})
		})
		if err != nil {
			return err
		}
		m, err := q.GetMerchantBrief(ctx, o.MerchantID)
		if err != nil {
			return err
		}
		if err := notification.Create(ctx, q, notification.New{UserID: m.OwnerID, Type: sqlc.NotificationTypeMerchant,
			Title: "Komplain dari pembeli", Body: "Ada komplain untuk pesanan " + o.Code + ". Tim SisaGuna akan menindaklanjuti.",
			Data: map[string]any{"order_id": o.ID}}); err != nil {
			return err
		}
		out = ComplaintDTO{ID: row.ID.String(), OrderID: o.ID.String(), Reason: in.Reason, Detail: detail, Status: string(row.Status),
			PhotoURLs: urls, CreatedAt: row.CreatedAt.Time.UTC()}
		return nil
	})
	if err != nil {
		return nil, wrap("complaint service create", err)
	}
	return &out, nil
}

// AdminList: semua komplain (terbaru dulu), bisa difilter status.
func (s *Service) AdminList(ctx context.Context, status string, page, limit int) ([]AdminComplaintDTO, int64, error) {
	p := sqlc.ListComplaintsAdminParams{Lim: int32(limit), Off: int32((page - 1) * limit)}
	if status != "" {
		p.Status = sqlc.NullComplaintStatus{ComplaintStatus: sqlc.ComplaintStatus(status), Valid: true}
	}
	rows, err := s.q.ListComplaintsAdmin(ctx, p)
	if err != nil {
		return nil, 0, fmt.Errorf("complaint service admin list: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	photos, err := s.q.ListComplaintMedia(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("complaint service admin list: %w", err)
	}
	byComplaint := map[uuid.UUID][]string{}
	for _, m := range photos {
		byComplaint[m.ComplaintID] = append(byComplaint[m.ComplaintID], s.urls.Private(m.MediaID.String()))
	}
	var total int64
	out := make([]AdminComplaintDTO, 0, len(rows))
	for _, r := range rows {
		total = r.Total
		d := AdminComplaintDTO{OrderCode: r.OrderCode, MerchantName: r.MerchantName, BuyerName: r.BuyerName,
			ComplaintDTO: ComplaintDTO{ID: r.ID.String(), OrderID: r.OrderID.String(), Reason: string(r.Reason), Detail: r.Detail,
				Status: string(r.Status), ResolutionNote: dbconv.Text(r.ResolutionNote), PhotoURLs: byComplaint[r.ID],
				CreatedAt: r.CreatedAt.Time.UTC(), ResolvedAt: dbconv.Time(r.ResolvedAt)}}
		if d.PhotoURLs == nil {
			d.PhotoURLs = []string{}
		}
		out = append(out, d)
	}
	return out, total, nil
}

// Update mengubah status (submitted -> reviewing -> resolved|rejected). Komplain yang sudah ditutup tidak bisa diubah.
// Pembeli diberi tahu lewat notifikasi.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateRequest) error {
	note := strings.TrimSpace(in.ResolutionNote)
	if in.Status != "reviewing" && note == "" {
		return ErrNoteRequired
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		c, err := q.LockComplaint(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if c.Status == sqlc.ComplaintStatusResolved || c.Status == sqlc.ComplaintStatusRejected {
			return ErrClosed
		}
		p := sqlc.UpdateComplaintParams{ID: id, Status: sqlc.ComplaintStatus(in.Status)}
		if note != "" {
			p.Note = pgtype.Text{String: note, Valid: true}
		}
		if err := q.UpdateComplaint(ctx, p); err != nil {
			return err
		}
		body := map[string]string{"reviewing": "Komplainmu sedang ditinjau.", "resolved": "Komplainmu selesai ditangani. " + note,
			"rejected": "Komplainmu tidak dapat diproses. " + note}[in.Status]
		return notification.Create(ctx, q, notification.New{UserID: c.BuyerID, Type: sqlc.NotificationTypeOrder,
			Title: "Update komplain", Body: body, Data: map[string]any{"order_id": c.OrderID}})
	})
	return wrap("complaint service update", err)
}

func wrap(ctx string, err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrOrderNotFound, ErrNotCompleted, ErrWindowClosed, ErrAlreadyComplaint, ErrNotFound, ErrClosed, media.ErrInvalidMedia} {
		if errors.Is(err, known) {
			return err
		}
	}
	return fmt.Errorf("%s: %w", ctx, err)
}
