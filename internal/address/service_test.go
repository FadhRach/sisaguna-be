package address

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func f(v float64) *float64 { return &v }
func str(s string) *string { return &s }

func newUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `INSERT INTO users (name,email,phone,password_hash) VALUES ('T',$1,'81234567890','x') RETURNING id`,
		uuid.NewString()+"@x.co").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAddressService(t *testing.T) {
	pool := testutil.NewPool(t)
	s := NewService(NewRepository(pool))
	ctx := context.Background()
	me, other := newUser(t, pool), newUser(t, pool)

	home, err := s.Create(ctx, me, CreateRequest{Label: " Rumah ", FullAddress: "Jl. Senopati 1", Lat: f(-6.2433), Lng: f(106.7989)})
	if err != nil {
		t.Fatal(err)
	}
	if home.Label != "Rumah" || home.Lat != -6.2433 || home.Lng != 106.7989 {
		t.Errorf("lat/lng order or trimming wrong: %+v", home)
	}

	t.Run("update is partial; lat without lng is rejected", func(t *testing.T) {
		out, err := s.Update(ctx, uuid.MustParse(home.ID), me, UpdateRequest{Note: str("lantai 2")})
		if err != nil || out.Note != "lantai 2" || out.Label != "Rumah" || out.Lat != -6.2433 {
			t.Errorf("%+v %v", out, err)
		}
		if _, err := s.Update(ctx, uuid.MustParse(home.ID), me, UpdateRequest{Lat: f(1)}); !errors.Is(err, ErrPartialPoint) {
			t.Errorf("want ErrPartialPoint, got %v", err)
		}
		moved, err := s.Update(ctx, uuid.MustParse(home.ID), me, UpdateRequest{Lat: f(-6.3), Lng: f(106.8)})
		if err != nil || moved.Lat != -6.3 || moved.Lng != 106.8 {
			t.Errorf("%+v %v", moved, err)
		}
	})

	t.Run("other users cannot see, change, select, or delete it", func(t *testing.T) {
		id := uuid.MustParse(home.ID)
		if l, _ := s.List(ctx, other); len(l) != 0 {
			t.Errorf("leak: %v", l)
		}
		if _, err := s.Update(ctx, id, other, UpdateRequest{Note: str("x")}); !errors.Is(err, ErrNotFound) {
			t.Errorf("update: %v", err)
		}
		if err := s.Select(ctx, other, &id); !errors.Is(err, ErrNotFound) {
			t.Errorf("select: %v", err)
		}
		if err := s.Delete(ctx, id, other); !errors.Is(err, ErrNotFound) {
			t.Errorf("delete: %v", err)
		}
	})

	t.Run("deleting the selected address falls back to GPS (null)", func(t *testing.T) {
		id := uuid.MustParse(home.ID)
		if err := s.Select(ctx, me, &id); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, id, me); err != nil {
			t.Fatal(err)
		}
		var sel *uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT selected_address_id FROM users WHERE id=$1`, me).Scan(&sel); err != nil || sel != nil {
			t.Errorf("selected = %v, err = %v", sel, err)
		}
		if err := s.Select(ctx, me, nil); err != nil {
			t.Errorf("select nil: %v", err)
		}
	})

	t.Run("limit per user", func(t *testing.T) {
		u := newUser(t, pool)
		for i := 0; i < maxPerUser; i++ {
			if _, err := s.Create(ctx, u, CreateRequest{Label: "A", FullAddress: "B", Lat: f(0), Lng: f(0)}); err != nil {
				t.Fatalf("create %d: %v", i, err)
			}
		}
		if _, err := s.Create(ctx, u, CreateRequest{Label: "A", FullAddress: "B", Lat: f(0), Lng: f(0)}); !errors.Is(err, ErrLimit) {
			t.Errorf("want ErrLimit, got %v", err)
		}
	})
}
