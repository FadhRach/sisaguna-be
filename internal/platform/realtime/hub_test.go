package realtime

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/platform/events"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

type fakePeers struct{ peer uuid.UUID }

func (f fakePeers) Peer(_ context.Context, user, _ uuid.UUID) (uuid.UUID, bool) {
	return f.peer, user != f.peer
}

type rig struct {
	t    *testing.T
	hub  *Hub
	url  string
	emit func(typ string, data any, users []uuid.UUID, topic string)
}

func newRig(t *testing.T, peer uuid.UUID) *rig {
	pool := testutil.NewPool(t)
	hub := NewHub(fakePeers{peer: peer})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	pass := func(c *gin.Context) {}
	hub.Routes(router, router.Group("/api/v1"), pass, pass)
	srv := httptest.NewServer(router)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); srv.Close() })
	go hub.Run(ctx, pool.Config().ConnConfig.Copy())

	// tunggu sampai LISTEN benar-benar aktif
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE query = 'LISTEN sg_events'`).Scan(&n)
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("LISTEN tidak aktif")
		}
		time.Sleep(50 * time.Millisecond)
	}
	emit := func(typ string, data any, users []uuid.UUID, topic string) {
		t.Helper()
		err := database.InTx(ctx, pool, func(tx pgx.Tx) error {
			return events.Emit(ctx, sqlc.New(pool).WithTx(tx), typ, data, users, topic)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return &rig{t: t, hub: hub, url: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?ticket=", emit: emit}
}

func (r *rig) dial(user uuid.UUID) *websocket.Conn {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, r.url+r.hub.tickets.issue(user), nil)
	if err != nil {
		r.t.Fatalf("dial: %v", err)
	}
	r.t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func read(t *testing.T, conn *websocket.Conn, wait time.Duration) (outbound, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, b, err := conn.Read(ctx)
	if err != nil {
		return outbound{}, false
	}
	var o outbound
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatalf("pesan rusak %q: %v", b, err)
	}
	return o, true
}

func send(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

// expect membaca pesan berikutnya dan memastikan tipenya. Urutan pesan terjaga, jadi event
// "sentinel" yang dikirim sesudahnya dipakai untuk membuktikan sebuah event TIDAK diterima
// (membatalkan Read lewat timeout akan menutup koneksi, jadi tidak dipakai untuk cek negatif).
func expect(t *testing.T, conn *websocket.Conn, typ string) outbound {
	t.Helper()
	got, ok := read(t, conn, 3*time.Second)
	if !ok || got.Type != typ {
		t.Fatalf("mau %q, dapat %+v ok=%v", typ, got, ok)
	}
	return got
}

func TestHubDeliversEvents(t *testing.T) {
	alice, bob := uuid.New(), uuid.New()
	r := newRig(t, alice) // lawan bicara "typing" selalu alice
	a, b := r.dial(alice), r.dial(bob)

	// event per pengguna: bob tidak menerima event alice
	r.emit("order.updated", map[string]any{"order_id": "o1", "status": "completed"}, []uuid.UUID{alice}, "")
	r.emit("sentinel", nil, []uuid.UUID{bob}, "")
	got := expect(t, a, "order.updated")
	if !strings.Contains(string(got.Data), `"completed"`) || got.At.IsZero() {
		t.Errorf("isi event: %+v", got)
	}
	expect(t, b, "sentinel")

	// topic publik: harus subscribe dulu; topic aneh ditolak
	topic := "listing:" + uuid.NewString()
	r.emit("listing.stock_changed", map[string]any{"stock": 3}, nil, topic)
	r.emit("sentinel", nil, []uuid.UUID{alice}, "")
	expect(t, a, "sentinel") // event topic sebelum subscribe tidak diterima

	send(t, a, map[string]string{"type": "subscribe", "topic": "chat:semua"})
	expect(t, a, "error")
	send(t, a, map[string]string{"type": "subscribe", "topic": topic})
	time.Sleep(200 * time.Millisecond) // subscribe diproses di goroutine baca
	r.emit("listing.stock_changed", map[string]any{"stock": 2}, nil, topic)
	expect(t, a, "listing.stock_changed")

	send(t, a, map[string]string{"type": "unsubscribe", "topic": topic})
	time.Sleep(200 * time.Millisecond)
	r.emit("listing.stock_changed", map[string]any{"stock": 1}, nil, topic)
	r.emit("sentinel", nil, []uuid.UUID{alice}, "")
	expect(t, a, "sentinel")

	// typing dari bob diteruskan ke lawan bicaranya (alice)
	send(t, b, map[string]string{"type": "typing", "conversation_id": uuid.NewString()})
	expect(t, a, "typing")
}

func TestTicketIsSingleUse(t *testing.T) {
	r := newRig(t, uuid.New())
	user := uuid.New()
	tk := r.hub.tickets.issue(user)
	if got, ok := r.hub.tickets.redeem(tk); !ok || got != user {
		t.Fatalf("redeem pertama: %v %v", got, ok)
	}
	if _, ok := r.hub.tickets.redeem(tk); ok {
		t.Error("tiket tidak boleh dipakai dua kali")
	}
	expired := r.hub.tickets.issue(user)
	r.hub.tickets.now = func() time.Time { return time.Now().Add(ticketTTL + time.Second) }
	if _, ok := r.hub.tickets.redeem(expired); ok {
		t.Error("tiket kedaluwarsa harus ditolak")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, resp, err := websocket.Dial(ctx, r.url+"salah", nil); err == nil || resp == nil || resp.StatusCode != 401 {
		t.Errorf("tiket palsu harus 401: %v %v", resp, err)
	}
}

func TestMaxConnectionsPerUser(t *testing.T) {
	r := newRig(t, uuid.New())
	user := uuid.New()
	first := r.dial(user)
	for range maxConnsPerUser {
		r.dial(user)
	}
	// koneksi tertua ditutup ketika koneksi ke-6 masuk
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, _, err := first.Read(ctx); err == nil {
		t.Error("koneksi tertua seharusnya ditutup")
	}
	r.hub.mu.Lock()
	n := len(r.hub.users[user])
	r.hub.mu.Unlock()
	if n != maxConnsPerUser {
		t.Errorf("koneksi aktif = %d, mau %d", n, maxConnsPerUser)
	}
}
