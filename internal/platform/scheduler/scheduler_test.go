package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestOnlyOneInstanceRunsJobs(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()
	runs := 0
	s := New(pool, time.Minute, Job{Name: "count", Run: func(context.Context) error { runs++; return nil }},
		Job{Name: "panics", Run: func(context.Context) error { panic("boom") }})

	// instance lain memegang lock
	other, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		t.Fatal(err)
	}
	if ran, err := s.RunOnce(ctx); err != nil || ran || runs != 0 {
		t.Fatalf("locked: ran=%v runs=%d err=%v", ran, runs, err)
	}

	_, _ = other.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey)
	other.Release()
	if ran, err := s.RunOnce(ctx); err != nil || !ran || runs != 1 {
		t.Fatalf("free: ran=%v runs=%d err=%v", ran, runs, err)
	}
	// lock dilepas setelah tick, panic di satu job tidak menghentikan scheduler
	if ran, _ := s.RunOnce(ctx); !ran || runs != 2 {
		t.Errorf("second tick: ran=%v runs=%d", ran, runs)
	}
}
