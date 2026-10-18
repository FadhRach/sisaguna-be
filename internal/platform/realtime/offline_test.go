package realtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDispatchCallsOfflineHookOnlyForUsersWithoutConnection(t *testing.T) {
	h := NewHub(nil)
	online, away := uuid.New(), uuid.New()
	h.users[online] = []*client{newClient(h, online, nil, func() {})}

	got := make(chan uuid.UUID, 2)
	h.OnOffline(func(u uuid.UUID, typ string, _ json.RawMessage) {
		if typ == "notification.created" {
			got <- u
		}
	})
	payload, _ := json.Marshal(map[string]any{"type": "notification.created", "user_ids": []uuid.UUID{online, away}, "at": time.Now(), "data": map[string]any{}})
	h.dispatch(string(payload))

	select {
	case u := <-got:
		if u != away {
			t.Fatalf("hook untuk %s, want %s", u, away)
		}
	case <-time.After(time.Second):
		t.Fatal("hook offline tidak dipanggil")
	}
	select {
	case u := <-got:
		t.Fatalf("hook kedua untuk %s; user online tidak boleh di-push", u)
	case <-time.After(150 * time.Millisecond):
	}
}
