// Package realtime: hub WebSocket. Hub hanya memberi tahu klien bahwa sesuatu berubah; data resmi
// tetap diambil lewat REST. Event datang dari Postgres (LISTEN sg_events), dikirim oleh transaksi
// yang mengubah datanya (package events).
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/events"
)

const (
	maxConnsPerUser = 5
	maxTopics       = 20
	sendBuffer      = 32
	maxInbound      = 4096
)

// topicPattern: satu-satunya topic publik adalah listing:<uuid>.
var topicPattern = regexp.MustCompile(`^listing:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Peers menjawab siapa lawan bicara di sebuah percakapan (untuk meneruskan "typing").
type Peers interface {
	Peer(ctx context.Context, user, conversation uuid.UUID) (uuid.UUID, bool)
}

type Hub struct {
	peers   Peers
	tickets *tickets

	offline func(user uuid.UUID, typ string, data json.RawMessage)

	mu     sync.Mutex
	users  map[uuid.UUID][]*client // tertua di depan
	topics map[string]map[*client]struct{}
}

func NewHub(peers Peers) *Hub {
	return &Hub{peers: peers, tickets: newTickets(), users: map[uuid.UUID][]*client{}, topics: map[string]map[*client]struct{}{}}
}

// OnOffline mendaftarkan hook untuk penerima event yang tidak punya koneksi WebSocket (dipakai push FCM).
// Dipanggil di goroutine sendiri; atur sebelum Run.
func (h *Hub) OnOffline(fn func(user uuid.UUID, typ string, data json.RawMessage)) { h.offline = fn }

// add mendaftarkan klien; kalau akun sudah punya maxConnsPerUser koneksi, yang tertua ditutup.
func (h *Hub) add(c *client) {
	h.mu.Lock()
	conns := h.users[c.user]
	var evict *client
	if len(conns) >= maxConnsPerUser {
		evict, conns = conns[0], conns[1:]
	}
	h.users[c.user] = append(conns, c)
	h.mu.Unlock()
	if evict != nil {
		evict.kill()
	}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.users[c.user]
	for i, x := range conns {
		if x == c {
			h.users[c.user] = append(conns[:i:i], conns[i+1:]...)
			break
		}
	}
	if len(h.users[c.user]) == 0 {
		delete(h.users, c.user)
	}
	for t := range c.topics {
		delete(h.topics[t], c)
		if len(h.topics[t]) == 0 {
			delete(h.topics, t)
		}
	}
}

// subscribe mengembalikan false kalau topic ditolak (bentuk salah atau sudah 20 topic).
func (h *Hub) subscribe(c *client, topic string) bool {
	if !topicPattern.MatchString(topic) {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := c.topics[topic]; !ok && len(c.topics) >= maxTopics {
		return false
	}
	c.topics[topic] = struct{}{}
	if h.topics[topic] == nil {
		h.topics[topic] = map[*client]struct{}{}
	}
	h.topics[topic][c] = struct{}{}
	return true
}

func (h *Hub) unsubscribe(c *client, topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(c.topics, topic)
	delete(h.topics[topic], c)
	if len(h.topics[topic]) == 0 {
		delete(h.topics, topic)
	}
}

// outbound adalah bentuk pesan server ke HP.
type outbound struct {
	Type string          `json:"type"`
	At   time.Time       `json:"at"`
	Data json.RawMessage `json:"data"`
}

// inbound adalah payload sg_events; Data dibiarkan mentah karena hub tidak perlu membacanya.
type inbound struct {
	Type    string          `json:"type"`
	UserIDs []uuid.UUID     `json:"user_ids"`
	Topic   string          `json:"topic"`
	At      time.Time       `json:"at"`
	Data    json.RawMessage `json:"data"`
}

// dispatch meneruskan satu payload sg_events ke penerimanya.
func (h *Hub) dispatch(payload string) {
	var ev inbound
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		slog.Error("event realtime rusak", "err", err)
		return
	}
	msg, _ := json.Marshal(outbound{Type: ev.Type, At: ev.At, Data: ev.Data})
	h.mu.Lock()
	var targets []*client
	var away []uuid.UUID
	for _, u := range ev.UserIDs {
		if len(h.users[u]) == 0 {
			away = append(away, u)
		}
		targets = append(targets, h.users[u]...)
	}
	if ev.Topic != "" {
		for c := range h.topics[ev.Topic] {
			targets = append(targets, c)
		}
	}
	h.mu.Unlock()
	for _, c := range targets {
		c.enqueue(msg)
	}
	if h.offline != nil {
		// shortcut: satu goroutine per penerima offline tanpa antrean, cukup untuk satu instance; pakai worker pool bila event ribuan per detik
		for _, u := range away {
			go h.offline(u, ev.Type, ev.Data)
		}
	}
}

// typing meneruskan indikator mengetik ke lawan bicara; tidak menyentuh database selain cek peserta.
func (h *Hub) typing(ctx context.Context, from uuid.UUID, conversation uuid.UUID) {
	peer, ok := h.peers.Peer(ctx, from, conversation)
	if !ok {
		return
	}
	data, _ := json.Marshal(map[string]any{"conversation_id": conversation})
	payload, _ := json.Marshal(events.Event{Type: "typing", UserIDs: []uuid.UUID{peer}, At: time.Now().UTC(), Data: json.RawMessage(data)})
	h.dispatch(string(payload))
}
