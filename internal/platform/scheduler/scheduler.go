// Package scheduler menjalankan job periodik di bawah pg_try_advisory_lock supaya tidak jalan ganda
// kalau ada lebih dari satu instance (CLAUDE.md aturan 8).
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const lockKey int64 = 7_300_000_001 // konstanta bebas, unik untuk scheduler SisaGuna

type Job struct {
	Name string
	Run  func(ctx context.Context) error
}

type Scheduler struct {
	pool     *pgxpool.Pool
	interval time.Duration
	jobs     []Job
}

func New(pool *pgxpool.Pool, interval time.Duration, jobs ...Job) *Scheduler {
	return &Scheduler{pool: pool, interval: interval, jobs: jobs}
}

// Run berjalan sampai ctx dibatalkan (shutdown).
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.RunOnce(ctx); err != nil {
				slog.Error("scheduler tick gagal", "err", err)
			}
		}
	}
}

// RunOnce menjalankan semua job kalau lock didapat. ran=false berarti instance lain sedang menjalankannya.
func (s *Scheduler) RunOnce(ctx context.Context) (ran bool, err error) {
	conn, err := s.pool.Acquire(ctx) // lock bersifat per-sesi, jadi koneksi dipegang selama tick
	if err != nil {
		return false, fmt.Errorf("scheduler acquire: %w", err)
	}
	defer conn.Release()

	var got bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&got); err != nil {
		return false, fmt.Errorf("scheduler lock: %w", err)
	}
	if !got {
		return false, nil
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	for _, j := range s.jobs {
		s.run(ctx, j)
	}
	return true, nil
}

func (s *Scheduler) run(ctx context.Context, j Job) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("job panic", "job", j.Name, "panic", r)
		}
	}()
	if err := j.Run(ctx); err != nil {
		slog.Error("job gagal", "job", j.Name, "err", err)
	}
}
