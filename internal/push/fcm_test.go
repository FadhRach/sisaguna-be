package push

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type fakeTokens struct {
	mu      sync.Mutex
	tokens  []string
	deleted []string
}

func (f *fakeTokens) ListDeviceTokens(context.Context, uuid.UUID) ([]string, error) {
	return f.tokens, nil
}

func (f *fakeTokens) DeleteDeviceTokenValue(_ context.Context, t string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, t)
	return nil
}

func TestSend(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(t, key)})

	var oauthCalls int
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oauthCalls++
		r.ParseForm()
		parsed, err := jwt.Parse(r.Form.Get("assertion"), func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithValidMethods([]string{"RS256"}))
		if err != nil || !parsed.Valid {
			http.Error(w, "bad assertion", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-123", "expires_in": 3600})
	}))
	defer oauth.Close()

	var got []map[string]any
	fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" || !strings.HasSuffix(r.URL.Path, "/projects/proj-1/messages:send") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var body map[string]map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		got = append(got, body["message"])
		if body["message"]["token"] == "dead" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"status":"NOT_FOUND","details":[{"errorCode":"UNREGISTERED"}]}}`))
			return
		}
		w.Write([]byte(`{"name":"ok"}`))
	}))
	defer fcm.Close()

	creds, _ := json.Marshal(map[string]string{"project_id": "proj-1", "client_email": "sa@proj-1.iam.gserviceaccount.com", "private_key": string(pemKey), "token_uri": oauth.URL})
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, creds, 0o600); err != nil {
		t.Fatal(err)
	}
	tokens := &fakeTokens{tokens: []string{"alive", "dead"}}
	f, err := Load(path, "", tokens)
	if err != nil {
		t.Fatal(err)
	}
	f.sendURL = fcm.URL + "/v1/projects/%s/messages:send"

	data, _ := json.Marshal(map[string]any{"notification_id": "n1", "type": "order", "title": "Pesanan siap", "body": "Ambil sebelum 18.00"})
	f.OnOffline(uuid.New(), "notification.created", data)
	f.OnOffline(uuid.New(), "chat.message", data) // bukan notifikasi: diabaikan

	if len(got) != 2 {
		t.Fatalf("pesan terkirim = %d, want 2 (alive + dead)", len(got))
	}
	n := got[0]["notification"].(map[string]any)
	if got[0]["token"] != "alive" || n["title"] != "Pesanan siap" || n["body"] != "Ambil sebelum 18.00" {
		t.Errorf("message = %v", got[0])
	}
	if d := got[0]["data"].(map[string]any); d["notification_id"] != "n1" || d["type"] != "order" {
		t.Errorf("data = %v", d)
	}
	if len(tokens.deleted) != 1 || tokens.deleted[0] != "dead" {
		t.Errorf("deleted = %v, want [dead]", tokens.deleted)
	}
	if oauthCalls != 1 {
		t.Errorf("access token harus di-cache, oauth calls = %d", oauthCalls)
	}
	if time.Until(f.expiry) < 50*time.Minute {
		t.Errorf("expiry = %v", f.expiry)
	}
}

func TestLoadRejectsBadCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte(`{"project_id":"p","client_email":"e","private_key":"bukan pem"}`), 0o600)
	if _, err := Load(path, "", &fakeTokens{}); err == nil {
		t.Fatal("kunci rusak harus ditolak saat start")
	}
}

func mustPKCS8(t *testing.T, k *rsa.PrivateKey) []byte {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
