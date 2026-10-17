package realtime

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/google/uuid"
)

const ticketTTL = 30 * time.Second

// tickets menyimpan tiket WebSocket sekali pakai di memori. shortcut: satu instance API saja;
// kalau nanti lebih dari satu, pindahkan ke tabel/Redis.
type tickets struct {
	mu  sync.Mutex
	m   map[string]ticket
	now func() time.Time
}

type ticket struct {
	user    uuid.UUID
	expires time.Time
}

func newTickets() *tickets { return &tickets{m: map[string]ticket{}, now: time.Now} }

func (t *tickets) issue(user uuid.UUID) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for k, v := range t.m { // tiket kedaluwarsa dibuang setiap kali ada yang baru
		if now.After(v.expires) {
			delete(t.m, k)
		}
	}
	t.m[id] = ticket{user: user, expires: now.Add(ticketTTL)}
	return id
}

// redeem menghapus tiket (sekali pakai) dan mengembalikan pemiliknya kalau belum kedaluwarsa.
func (t *tickets) redeem(id string) (uuid.UUID, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tk, ok := t.m[id]
	delete(t.m, id)
	if !ok || t.now().After(tk.expires) {
		return uuid.Nil, false
	}
	return tk.user, true
}
