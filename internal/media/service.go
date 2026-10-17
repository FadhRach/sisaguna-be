package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

var (
	ErrNotFound     = errors.New("file tidak ditemukan")
	ErrVideoPurpose = errors.New("video hanya boleh untuk purpose=listing")
)

type Service struct {
	repo   *Repository
	disk   *pmedia.Disk
	urls   pmedia.URLs
	remote pmedia.Remote // nil = cadangan R2 mati (development)
}

// WithRemote mengaktifkan salinan ke object storage (SyncToRemote) dan penghapusan di sana saat file yatim dibersihkan.
func (s *Service) WithRemote(r pmedia.Remote) *Service {
	s.remote = r
	return s
}

func NewService(repo *Repository, disk *pmedia.Disk, urls pmedia.URLs) *Service {
	return &Service{repo: repo, disk: disk, urls: urls}
}

// Upload memproses gambar lalu menyimpannya. purpose=complaint otomatis privat (tanpa thumbnail publik).
// Upload ulang file yang sama persis oleh akun yang sama mengembalikan baris lama.
func (s *Service) Upload(ctx context.Context, owner uuid.UUID, purpose string, data []byte) (*MediaDTO, error) {
	vis := pmedia.Public
	if purpose == "complaint" {
		vis = pmedia.Private
	}
	img, err := pmedia.ProcessImage(data)
	if err != nil {
		return nil, err
	}
	sha, rel, thumbRel := pmedia.Paths(vis, img.Main)
	if err := s.disk.Write(vis, rel, img.Main); err != nil {
		return nil, fmt.Errorf("media service upload: %w", err)
	}
	var thumb *string
	if vis == pmedia.Public {
		if err := s.disk.Write(vis, thumbRel, img.Thumb); err != nil {
			return nil, fmt.Errorf("media service upload: %w", err)
		}
		thumb = &thumbRel
	}
	row, err := s.repo.Upsert(ctx, owner, string(vis), len(img.Main), img.Width, img.Height, sha, rel, thumb)
	if err != nil {
		return nil, fmt.Errorf("media service upload: %w", err)
	}

	dto := &MediaDTO{ID: row.ID.String(), Kind: "image", Width: int(row.Width.Int32), Height: int(row.Height.Int32)}
	if vis == pmedia.Private {
		dto.URL = s.urls.Private(dto.ID)
	} else {
		dto.URL = s.urls.Public(row.Path)
		if row.ThumbPath.Valid {
			t := s.urls.Public(row.ThumbPath.String)
			dto.ThumbURL = &t
		}
	}
	return dto, nil
}

// UploadVideoFor menolak video untuk tujuan selain listing, lalu menyimpannya.
func (s *Service) UploadVideoFor(ctx context.Context, owner uuid.UUID, purpose string, data []byte) (*MediaDTO, error) {
	if purpose != "listing" {
		return nil, ErrVideoPurpose
	}
	return s.UploadVideo(ctx, owner, data)
}

// UploadVideo menyimpan video MP4 (maks 30 detik, 30 MB) untuk listing. Metadata lokasi dibuang, video tidak di-transcode.
func (s *Service) UploadVideo(ctx context.Context, owner uuid.UUID, data []byte) (*MediaDTO, error) {
	v, err := pmedia.ProcessVideo(data)
	if err != nil {
		return nil, err
	}
	sha := pmedia.Hash(pmedia.Public, v.Data)
	rel := pmedia.VideoPath(sha)
	if err := s.disk.Write(pmedia.Public, rel, v.Data); err != nil {
		return nil, fmt.Errorf("media service upload video: %w", err)
	}
	row, err := s.repo.UpsertVideo(ctx, owner, len(v.Data), v.DurationMs, sha, rel)
	if err != nil {
		return nil, fmt.Errorf("media service upload video: %w", err)
	}
	ms := int(row.DurationMs.Int32)
	return &MediaDTO{ID: row.ID.String(), Kind: "video", URL: s.urls.Public(row.Path), DurationMs: &ms}, nil
}

// PrivateFile mengembalikan path disk file privat. Boleh dibuka pemilik file, pemilik toko yang
// dikomplain, atau admin. Selain itu dijawab "tidak ditemukan" supaya keberadaan file tidak bocor.
func (s *Service) PrivateFile(ctx context.Context, requester uuid.UUID, isAdmin bool, id uuid.UUID) (string, error) {
	m, err := s.repo.Get(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("media service private: %w", err)
	}
	if m.Visibility != "private" {
		return "", ErrNotFound
	}
	if m.OwnerID != requester && !isAdmin {
		ok, err := s.repo.q.MerchantOwnsComplaintMedia(ctx, sqlc.MerchantOwnsComplaintMediaParams{MediaID: id, UserID: requester})
		if err != nil {
			return "", fmt.Errorf("media service private: %w", err)
		}
		if !ok {
			return "", ErrNotFound
		}
	}
	return s.disk.FilePath(pmedia.Private, m.Path), nil
}

// CleanupOrphans menghapus foto yang diunggah tapi tidak pernah dipakai lebih dari 24 jam
// (mis. mitra memilih foto lalu membatalkan listing). File fisik dihapus hanya kalau tidak
// dipakai baris media lain (hash isi yang sama bisa dimiliki beberapa akun).
func (s *Service) CleanupOrphans(ctx context.Context) error {
	rows, err := s.repo.q.ListOrphanMedia(ctx)
	if err != nil {
		return fmt.Errorf("media service cleanup: %w", err)
	}
	for _, m := range rows {
		vis := pmedia.Visibility(m.Visibility)
		if s.remote != nil && !m.SharedPath {
			if err := s.removeRemote(ctx, vis, m.Path, m.ThumbPath.String); err != nil {
				slog.Error("hapus file media di R2 gagal, dicoba lagi tick berikutnya", "media_id", m.ID, "err", err)
				continue
			}
		}
		if err := s.repo.q.DeleteMedia(ctx, m.ID); err != nil {
			slog.Error("hapus baris media yatim gagal", "media_id", m.ID, "err", err)
			continue
		}
		if m.SharedPath {
			continue
		}
		for _, rel := range []string{m.Path, m.ThumbPath.String} {
			if rel == "" {
				continue
			}
			if err := s.disk.Remove(vis, rel); err != nil {
				slog.Error("hapus file media yatim gagal", "media_id", m.ID, "err", err)
			}
		}
	}
	return nil
}

func (s *Service) removeRemote(ctx context.Context, vis pmedia.Visibility, rels ...string) error {
	for _, rel := range rels {
		if rel == "" {
			continue
		}
		if err := s.remote.Delete(ctx, pmedia.Key(vis, rel)); err != nil {
			return err
		}
	}
	return nil
}

// SyncToRemote menyalin file yang belum punya salinan (r2_synced_at NULL) ke object storage, 100 baris per tick.
// Satu file gagal tidak menghentikan yang lain; yang gagal dicoba lagi di tick berikutnya.
func (s *Service) SyncToRemote(ctx context.Context) error {
	if s.remote == nil {
		return nil
	}
	rows, err := s.repo.q.ListUnsyncedMedia(ctx)
	if err != nil {
		return fmt.Errorf("media service sync: %w", err)
	}
	for _, m := range rows {
		vis := pmedia.Visibility(m.Visibility)
		failed := false
		for _, rel := range []string{m.Path, m.ThumbPath.String} {
			if rel == "" {
				continue
			}
			if err := s.remote.Put(ctx, pmedia.Key(vis, rel), s.disk.FilePath(vis, rel)); err != nil {
				slog.Error("sinkron R2 gagal", "media_id", m.ID, "err", err)
				failed = true
				break
			}
		}
		if failed {
			continue
		}
		if err := s.repo.q.MarkMediaSynced(ctx, m.ID); err != nil {
			return fmt.Errorf("media service sync: %w", err)
		}
	}
	return nil
}
