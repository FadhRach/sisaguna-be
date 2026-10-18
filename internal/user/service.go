package user

import (
	"errors"
	"strings"
	"time"

	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	"github.com/FadhRach/sisaguna-be/internal/platform/mailer"
)

var (
	ErrEmailTaken          = errors.New("email sudah terdaftar")
	ErrInvalidCredentials  = errors.New("email atau password salah")
	ErrInvalidRefreshToken = errors.New("refresh token tidak valid atau kedaluwarsa")
	ErrUserNotFound        = errors.New("pengguna tidak ditemukan")
	ErrWrongPassword       = errors.New("password saat ini salah")
	ErrInvalidMedia        = errors.New("foto tidak valid: harus foto (bukan video) publik milik akun ini")
)

// Service menangani sesi (session.go) dan profil (profile.go).
type Service struct {
	repo       *Repository
	tokens     *auth.Tokens
	passwords  *auth.Passwords
	refreshTTL time.Duration
	mailer     mailer.Sender // nil = tidak mengirim email (kode reset tidak terkirim)
}

// WithMailer mengaktifkan pengiriman email kode lupa password.
func (s *Service) WithMailer(m mailer.Sender) *Service {
	s.mailer = m
	return s
}

func NewService(repo *Repository, tokens *auth.Tokens, passwords *auth.Passwords, refreshTTL time.Duration) *Service {
	return &Service{repo: repo, tokens: tokens, passwords: passwords, refreshTTL: refreshTTL}
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
