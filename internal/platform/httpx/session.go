package httpx

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AuthState adalah keadaan akun di database. IsAdmin dari sini yang berlaku, bukan klaim di token.
type AuthState struct {
	IsAdmin    bool
	ValidAfter time.Time // access token yang diterbitkan sebelum ini (dibulatkan ke detik) ditolak
}

// StateLookup membaca keadaan akun; ok=false berarti akun sudah tidak ada.
type StateLookup func(ctx context.Context, id uuid.UUID) (state AuthState, ok bool, err error)

// SessionChecker memeriksa tiap request terotentikasi ke database lewat cache singkat (ttl), sehingga akun yang
// dihapus, admin yang dicabut, atau sesi yang dibatalkan (ganti/reset password) berhenti berlaku dalam hitungan detik,
// bukan menunggu token 15 menit kedaluwarsa. shortcut: cache di memori satu proses, cukup untuk satu instance API.
type SessionChecker struct {
	lookup StateLookup
	ttl    time.Duration
	now    func() time.Time

	mu    sync.Mutex
	cache map[uuid.UUID]cached
}

type cached struct {
	state AuthState
	ok    bool
	at    time.Time
}

func NewSessionChecker(lookup StateLookup, ttl time.Duration) *SessionChecker {
	return &SessionChecker{lookup: lookup, ttl: ttl, now: time.Now, cache: map[uuid.UUID]cached{}}
}

// Check mengembalikan keadaan akun. Kesalahan database ditolak (fail closed) supaya token tidak lolos saat DB bermasalah.
func (s *SessionChecker) Check(ctx context.Context, id uuid.UUID) (AuthState, bool, error) {
	now := s.now()
	s.mu.Lock()
	if c, ok := s.cache[id]; ok && now.Sub(c.at) < s.ttl {
		s.mu.Unlock()
		return c.state, c.ok, nil
	}
	s.mu.Unlock()

	state, ok, err := s.lookup(ctx, id)
	if err != nil {
		return AuthState{}, false, err
	}
	s.mu.Lock()
	if len(s.cache) > 10_000 { // sapu sederhana agar tidak tumbuh tanpa batas
		for k, c := range s.cache {
			if now.Sub(c.at) > s.ttl {
				delete(s.cache, k)
			}
		}
	}
	s.cache[id] = cached{state: state, ok: ok, at: now}
	s.mu.Unlock()
	return state, ok, nil
}
