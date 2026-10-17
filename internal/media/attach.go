package media

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
)

var ErrInvalidMedia = errors.New("file media tidak valid: harus milikmu, sesuai jenis, dan belum dipakai")

// Attach memastikan semua media milik owner, berjenis kind dan vis, dan belum dipakai, lalu
// menandainya terpakai. link mencatat relasi (urutan = posisi). Pakai Queries yang terikat transaksi:
// pemakaian ganda id yang sama otomatis ditolak karena attached_at sudah terisi.
func Attach(ctx context.Context, q *sqlc.Queries, owner uuid.UUID, kind sqlc.MediaKind, vis sqlc.MediaVisibility, ids []uuid.UUID,
	link func(id uuid.UUID, pos int16) error) error {
	for i, id := range ids {
		m, err := q.GetMedia(ctx, id)
		if err != nil {
			return ErrInvalidMedia // tidak ada (atau error baca: tetap ditolak)
		}
		if m.OwnerID != owner || m.Kind != kind || m.Visibility != vis || m.AttachedAt.Valid {
			return ErrInvalidMedia
		}
		if err := link(id, int16(i)); err != nil {
			return err
		}
		if err := q.AttachMedia(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
