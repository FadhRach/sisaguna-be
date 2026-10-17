package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func newService(t *testing.T) (*Service, *auth.Tokens, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t)
	pw, err := auth.NewPasswords()
	if err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokens("test-secret-test-secret-test-secret-123", 15*time.Minute)
	return NewService(NewRepository(pool), tokens, pw, time.Hour), tokens, pool
}

func register(t *testing.T, s *Service, email string) *Session {
	t.Helper()
	sess, err := s.Register(context.Background(), RegisterRequest{
		Name: "Clara", Email: email, Phone: "81234567890", Password: "Password123!", AccountType: "buyer",
	}, "test")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return sess
}

func TestAuthFlow(t *testing.T) {
	s, tokens, pool := newService(t)
	ctx := context.Background()

	t.Run("register normalizes email, rejects duplicate case-insensitively", func(t *testing.T) {
		sess := register(t, s, "  Budi@Gmail.com ")
		if sess.User.Email != "budi@gmail.com" {
			t.Errorf("email = %q", sess.User.Email)
		}
		_, err := s.Register(ctx, RegisterRequest{Name: "B", Email: "BUDI@gmail.com", Phone: "81234567890", Password: "Password123!", AccountType: "buyer"}, "")
		if !errors.Is(err, ErrEmailTaken) {
			t.Errorf("want ErrEmailTaken, got %v", err)
		}
	})

	t.Run("login wrong password and unknown email return the same error", func(t *testing.T) {
		register(t, s, "sari@gmail.com")
		_, e1 := s.Login(ctx, LoginRequest{Email: "sari@gmail.com", Password: "salah-salah"}, "")
		_, e2 := s.Login(ctx, LoginRequest{Email: "tidak-ada@gmail.com", Password: "salah-salah"}, "")
		if !errors.Is(e1, ErrInvalidCredentials) || !errors.Is(e2, ErrInvalidCredentials) {
			t.Errorf("got %v / %v", e1, e2)
		}
		if _, err := s.Login(ctx, LoginRequest{Email: "SARI@gmail.com", Password: "Password123!"}, ""); err != nil {
			t.Errorf("valid login: %v", err)
		}
	})

	t.Run("refresh rotates; reusing old token revokes the whole family", func(t *testing.T) {
		first := register(t, s, "andi@gmail.com")
		second, err := s.Refresh(ctx, first.RefreshToken, "")
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if second.RefreshToken == first.RefreshToken {
			t.Fatal("refresh token not rotated")
		}
		// token lama dipakai lagi -> ditolak dan family dicabut
		if _, err := s.Refresh(ctx, first.RefreshToken, ""); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("reuse: want ErrInvalidRefreshToken, got %v", err)
		}
		if _, err := s.Refresh(ctx, second.RefreshToken, ""); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("family should be revoked, got %v", err)
		}
	})

	t.Run("logout revokes token, only for its owner", func(t *testing.T) {
		a := register(t, s, "maya@gmail.com")
		b := register(t, s, "rina@gmail.com")
		if err := s.Logout(ctx, b.User.ID, a.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("foreign logout: %v", err)
		}
		if err := s.Logout(ctx, a.User.ID, a.RefreshToken); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if _, err := s.Refresh(ctx, a.RefreshToken, ""); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("refresh after logout: %v", err)
		}
	})

	t.Run("me returns profile; unknown id is not found", func(t *testing.T) {
		sess := register(t, s, "me@gmail.com")
		p, err := s.Me(ctx, sess.User.ID)
		if err != nil || p.Email != "me@gmail.com" || p.UnreadNotifications != 0 {
			t.Errorf("me: %+v %v", p, err)
		}
		if _, err := s.Me(ctx, uuid.New()); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("unknown: %v", err)
		}
	})

	t.Run("access token: HS256 accepted, other algorithms rejected", func(t *testing.T) {
		sess := register(t, s, "jwt@gmail.com")
		if _, err := tokens.Parse(sess.AccessToken); err != nil {
			t.Errorf("valid token: %v", err)
		}
		hs512, _ := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.RegisteredClaims{
			Subject: sess.User.ID.String(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		}).SignedString([]byte("test-secret-test-secret-test-secret-123"))
		if _, err := tokens.Parse(hs512); err == nil {
			t.Error("HS512 token must be rejected")
		}
		none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
			Subject: sess.User.ID.String(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		}).SignedString(jwt.UnsafeAllowNoneSignatureType)
		if _, err := tokens.Parse(none); err == nil {
			t.Error("alg=none token must be rejected")
		}
	})

	t.Run("update profile: partial, email collision, photo attach and detach", func(t *testing.T) {
		a := register(t, s, "upd-a@gmail.com")
		register(t, s, "upd-b@gmail.com")
		name := "  Nama Baru "
		if err := s.UpdateProfile(ctx, a.User.ID, UpdateProfileRequest{Name: &name}); err != nil {
			t.Fatalf("update name: %v", err)
		}
		p, _ := s.Me(ctx, a.User.ID)
		if p.Name != "Nama Baru" || p.Phone != "81234567890" {
			t.Errorf("partial update wrong: %q %q", p.Name, p.Phone)
		}
		taken := "UPD-B@gmail.com"
		if err := s.UpdateProfile(ctx, a.User.ID, UpdateProfileRequest{Email: &taken}); !errors.Is(err, ErrEmailTaken) {
			t.Errorf("want ErrEmailTaken, got %v", err)
		}

		mediaID := insertMedia(t, pool, a.User.ID, "public", "m1")
		other := insertMedia(t, pool, otherUser(t, s), "public", "m2")
		priv := insertMedia(t, pool, a.User.ID, "private", "m3")
		for _, bad := range []uuid.UUID{other, priv, uuid.New()} {
			in := UpdateProfileRequest{PhotoMediaID: Nullable[uuid.UUID]{Value: bad, Set: true, Valid: true}}
			if err := s.UpdateProfile(ctx, a.User.ID, in); !errors.Is(err, ErrInvalidMedia) {
				t.Errorf("media %s: want ErrInvalidMedia, got %v", bad, err)
			}
		}
		set := UpdateProfileRequest{PhotoMediaID: Nullable[uuid.UUID]{Value: mediaID, Set: true, Valid: true}}
		if err := s.UpdateProfile(ctx, a.User.ID, set); err != nil {
			t.Fatalf("set photo: %v", err)
		}
		p, _ = s.Me(ctx, a.User.ID)
		if !p.PhotoPath.Valid || attached(t, pool, mediaID) == false {
			t.Error("photo should be set and media attached")
		}
		clear := UpdateProfileRequest{PhotoMediaID: Nullable[uuid.UUID]{Set: true}}
		if err := s.UpdateProfile(ctx, a.User.ID, clear); err != nil {
			t.Fatalf("clear photo: %v", err)
		}
		p, _ = s.Me(ctx, a.User.ID)
		if p.PhotoPath.Valid || attached(t, pool, mediaID) {
			t.Error("photo should be cleared and media detached")
		}
	})

	t.Run("change password revokes other sessions and keeps this device logged in", func(t *testing.T) {
		old := register(t, s, "pw@gmail.com")
		if _, err := s.ChangePassword(ctx, old.User.ID, "salah-salah", "PasswordBaru1", ""); !errors.Is(err, ErrWrongPassword) {
			t.Fatalf("want ErrWrongPassword, got %v", err)
		}
		fresh, err := s.ChangePassword(ctx, old.User.ID, "Password123!", "PasswordBaru1", "")
		if err != nil {
			t.Fatalf("change: %v", err)
		}
		if _, err := s.Refresh(ctx, old.RefreshToken, ""); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("old refresh token must be revoked, got %v", err)
		}
		if _, err := s.Refresh(ctx, fresh.RefreshToken, ""); err != nil {
			t.Errorf("new session must work: %v", err)
		}
		if _, err := s.Login(ctx, LoginRequest{Email: "pw@gmail.com", Password: "Password123!"}, ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("old password must stop working, got %v", err)
		}
		if _, err := s.Login(ctx, LoginRequest{Email: "pw@gmail.com", Password: "PasswordBaru1"}, ""); err != nil {
			t.Errorf("new password: %v", err)
		}
	})
}

func otherUser(t *testing.T, s *Service) uuid.UUID {
	t.Helper()
	return register(t, s, uuid.NewString()+"@gmail.com").User.ID
}

func insertMedia(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID, vis, sha string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `INSERT INTO media (owner_id, kind, visibility, mime, size_bytes, sha256, path)
		VALUES ($1,'image',$2::media_visibility,'image/jpeg',10,$3,'ab/'||$3||'.jpg') RETURNING id`, owner, vis, sha+uuid.NewString()).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func attached(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT attached_at IS NOT NULL FROM media WHERE id=$1`, id).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}
