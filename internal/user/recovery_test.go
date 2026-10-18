package user

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"
)

type captureMailer struct{ ch chan string }

func (m captureMailer) Send(_ context.Context, _, _, body string) error {
	m.ch <- body
	return nil
}

var codeRe = regexp.MustCompile(`\d{6}`)

func waitCode(t *testing.T, m captureMailer) string {
	t.Helper()
	select {
	case body := <-m.ch:
		return codeRe.FindString(body)
	case <-time.After(3 * time.Second):
		t.Fatal("email reset tidak terkirim")
		return ""
	}
}

func TestPasswordReset(t *testing.T) {
	s, _, _ := newService(t)
	m := captureMailer{ch: make(chan string, 4)}
	s.WithMailer(m)
	ctx := context.Background()

	t.Run("kode benar mengganti password, mencabut sesi lama, membuka sesi baru", func(t *testing.T) {
		old := register(t, s, "reset@gmail.com")
		if err := s.ForgotPassword(ctx, "RESET@gmail.com"); err != nil {
			t.Fatal(err)
		}
		code := waitCode(t, m)
		sess, err := s.ResetPassword(ctx, "reset@gmail.com", code, "PasswordBaru456!", "test")
		if err != nil {
			t.Fatalf("reset: %v", err)
		}
		if _, err := s.Login(ctx, LoginRequest{Email: "reset@gmail.com", Password: "PasswordBaru456!"}, ""); err != nil {
			t.Errorf("login password baru: %v", err)
		}
		if _, err := s.Login(ctx, LoginRequest{Email: "reset@gmail.com", Password: "Password123!"}, ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("password lama harus ditolak, got %v", err)
		}
		if _, err := s.Refresh(ctx, old.RefreshToken, ""); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("refresh token lama harus dicabut, got %v", err)
		}
		if _, err := s.Refresh(ctx, sess.RefreshToken, ""); err != nil {
			t.Errorf("sesi baru harus jalan: %v", err)
		}
		if _, err := s.ResetPassword(ctx, "reset@gmail.com", code, "LainLagi789!", ""); !errors.Is(err, ErrInvalidResetCode) {
			t.Errorf("kode hanya sekali pakai, got %v", err)
		}
	})

	t.Run("email tak dikenal: tanpa error, tanpa email", func(t *testing.T) {
		if err := s.ForgotPassword(ctx, "hantu@gmail.com"); err != nil {
			t.Fatal(err)
		}
		select {
		case <-m.ch:
			t.Fatal("tidak boleh ada email")
		case <-time.After(200 * time.Millisecond):
		}
		if _, err := s.ResetPassword(ctx, "hantu@gmail.com", "123456", "PasswordBaru456!", ""); !errors.Is(err, ErrInvalidResetCode) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("5 kode salah mematikan kode, termasuk yang benar", func(t *testing.T) {
		register(t, s, "brute@gmail.com")
		if err := s.ForgotPassword(ctx, "brute@gmail.com"); err != nil {
			t.Fatal(err)
		}
		code := waitCode(t, m)
		wrong := "000000"
		if code == wrong {
			wrong = "000001"
		}
		for i := 0; i < maxResetAttempts; i++ {
			if _, err := s.ResetPassword(ctx, "brute@gmail.com", wrong, "PasswordBaru456!", ""); !errors.Is(err, ErrInvalidResetCode) {
				t.Fatalf("percobaan %d: %v", i, err)
			}
		}
		if _, err := s.ResetPassword(ctx, "brute@gmail.com", code, "PasswordBaru456!", ""); !errors.Is(err, ErrInvalidResetCode) {
			t.Errorf("kode benar setelah 5 salah harus ditolak, got %v", err)
		}
	})

	t.Run("permintaan kedua dalam 60 detik tidak mengirim email lagi", func(t *testing.T) {
		register(t, s, "spam@gmail.com")
		for i := 0; i < 2; i++ {
			if err := s.ForgotPassword(ctx, "spam@gmail.com"); err != nil {
				t.Fatal(err)
			}
		}
		waitCode(t, m)
		select {
		case <-m.ch:
			t.Fatal("email kedua tidak boleh terkirim")
		case <-time.After(200 * time.Millisecond):
		}
	})
}
