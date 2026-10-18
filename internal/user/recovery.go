package user

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/db/sqlc"

	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
)

const maxResetAttempts = 5

var ErrInvalidResetCode = errors.New("kode salah atau sudah kedaluwarsa")

// ForgotPassword mengirim kode 6 digit (berlaku 15 menit) ke email akun. Selalu sukses dari sisi pemanggil,
// ada atau tidaknya email tidak boleh terbaca dari respons. Kode lama dibatalkan; maks satu kode per 60 detik.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	u, err := s.repo.UserByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("user service forgot password: %w", err)
	}
	recent, err := s.repo.q.HasRecentReset(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("user service forgot password: %w", err)
	}
	if recent {
		return nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return fmt.Errorf("user service forgot password: %w", err)
	}
	code := fmt.Sprintf("%06d", n.Int64())
	err = s.repo.InTx(ctx, func(r *Repository) error {
		if err := r.q.InvalidateResets(ctx, u.ID); err != nil {
			return err
		}
		return r.q.CreateReset(ctx, sqlc.CreateResetParams{UserID: u.ID, CodeHash: resetHash(u.ID.String(), code)})
	})
	if err != nil {
		return fmt.Errorf("user service forgot password: %w", err)
	}
	if s.mailer != nil {
		// di goroutine supaya waktu respons tidak membedakan email yang terdaftar
		go func() {
			sendCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			body := "Kode untuk mengatur ulang password SisaGuna: " + code + "\n\nBerlaku 15 menit. Abaikan email ini kalau kamu tidak memintanya."
			if err := s.mailer.Send(sendCtx, u.Email, "Kode atur ulang password SisaGuna", body); err != nil {
				slog.Error("kirim email reset gagal", "err", err)
			}
		}()
	}
	return nil
}

// ResetPassword memverifikasi kode, mengganti password, mencabut semua sesi lama, dan membuka sesi baru.
// Kode salah dihitung; setelah 5 kali salah kode itu mati.
func (s *Service) ResetPassword(ctx context.Context, email, code, newPassword, userAgent string) (*Session, error) {
	hash, err := s.passwords.Hash(newPassword) // dihitung dulu supaya waktu respons tidak membedakan kasus
	if err != nil {
		return nil, fmt.Errorf("user service reset password: %w", err)
	}
	u, err := s.repo.UserByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidResetCode
	}
	if err != nil {
		return nil, fmt.Errorf("user service reset password: %w", err)
	}
	var sess *Session
	wrong := false
	err = s.repo.InTx(ctx, func(r *Repository) error {
		rec, err := r.q.GetActiveResetForUpdate(ctx, u.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			wrong = true
			return nil
		}
		if err != nil {
			return err
		}
		if rec.Attempts >= maxResetAttempts || subtle.ConstantTimeCompare([]byte(rec.CodeHash), []byte(resetHash(u.ID.String(), code))) != 1 {
			wrong = true
			return r.q.BumpResetAttempts(ctx, rec.ID) // di-commit; error dikembalikan setelah tx
		}
		if err := r.q.MarkResetUsed(ctx, rec.ID); err != nil {
			return err
		}
		if err := r.q.UpdatePasswordHash(ctx, sqlc.UpdatePasswordHashParams{ID: u.ID, PasswordHash: hash}); err != nil {
			return err
		}
		if err := r.q.RevokeAllUserRefreshTokens(ctx, u.ID); err != nil {
			return err
		}
		sess, err = s.openSession(ctx, r, u, uuid.New(), userAgent)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("user service reset password: %w", err)
	}
	if wrong {
		return nil, ErrInvalidResetCode
	}
	return sess, nil
}

func resetHash(userID, code string) string { return auth.HashToken(userID + ":" + code) }
