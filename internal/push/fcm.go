// Package push mengirim notifikasi ke HP lewat FCM HTTP v1 untuk pengguna yang tidak punya koneksi
// WebSocket. Dipanggil hub realtime setelah event notification.created (jadi sudah lewat commit dan preferensi).
package push

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
)

const scope = "https://www.googleapis.com/auth/firebase.messaging"

// Tokens memuat dan menghapus token perangkat; dipenuhi *sqlc.Queries.
type Tokens interface {
	ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]string, error)
	DeleteDeviceTokenValue(ctx context.Context, token string) error
}

var _ Tokens = (*sqlc.Queries)(nil)

type credentials struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type FCM struct {
	projectID string
	email     string
	key       *rsa.PrivateKey
	tokenURL  string
	sendURL   string // berisi %s untuk project id
	tokens    Tokens
	http      *http.Client

	mu     sync.Mutex
	access string
	expiry time.Time
}

// Load membaca service account JSON dari path. projectID kosong = ambil dari file.
func Load(path, projectID string, tokens Tokens) (*FCM, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("push load credentials: %w", err)
	}
	var c credentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("push parse credentials: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(c.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("push parse private key: %w", err)
	}
	if projectID == "" {
		projectID = c.ProjectID
	}
	if projectID == "" || c.ClientEmail == "" {
		return nil, errors.New("push load credentials: project_id atau client_email kosong")
	}
	if c.TokenURI == "" {
		c.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &FCM{projectID: projectID, email: c.ClientEmail, key: key, tokenURL: c.TokenURI,
		sendURL: "https://fcm.googleapis.com/v1/projects/%s/messages:send", tokens: tokens,
		http: &http.Client{Timeout: 10 * time.Second}}, nil
}

// OnOffline cocok dengan realtime.Hub.OnOffline: hanya notification.created yang dikirim sebagai push.
func (f *FCM) OnOffline(user uuid.UUID, typ string, data json.RawMessage) {
	if typ != "notification.created" {
		return
	}
	var d struct {
		ID    string `json:"notification_id"`
		Type  string `json:"type"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if json.Unmarshal(data, &d) != nil || d.Title == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := f.Send(ctx, user, d.Title, d.Body, map[string]string{"notification_id": d.ID, "type": d.Type}); err != nil {
		slog.Error("kirim push gagal", "user_id", user, "err", err)
	}
}

// Send mengirim ke semua perangkat user; token yang ditolak FCM (UNREGISTERED / tidak ditemukan) dihapus.
func (f *FCM) Send(ctx context.Context, user uuid.UUID, title, body string, data map[string]string) error {
	toks, err := f.tokens.ListDeviceTokens(ctx, user)
	if err != nil || len(toks) == 0 {
		return err
	}
	access, err := f.accessToken(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, tok := range toks {
		gone, err := f.sendOne(ctx, access, tok, title, body, data)
		if gone {
			_ = f.tokens.DeleteDeviceTokenValue(ctx, tok)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (f *FCM) sendOne(ctx context.Context, access, token, title, body string, data map[string]string) (gone bool, err error) {
	payload, _ := json.Marshal(map[string]any{"message": map[string]any{
		"token":        token,
		"notification": map[string]string{"title": title, "body": body},
		"data":         data,
		"android":      map[string]string{"priority": "HIGH"},
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(f.sendURL, f.projectID), bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("push send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 {
		return false, nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	gone = resp.StatusCode == http.StatusNotFound || strings.Contains(string(b), "UNREGISTERED")
	return gone, fmt.Errorf("push send: fcm status %d", resp.StatusCode)
}

// accessToken menukar JWT service account dengan access token OAuth, di-cache sampai mendekati kedaluwarsa.
func (f *FCM) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.access != "" && time.Now().Before(f.expiry) {
		return f.access, nil
	}
	now := time.Now()
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": f.email, "scope": scope, "aud": f.tokenURL, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString(f.key)
	if err != nil {
		return "", fmt.Errorf("push sign assertion: %w", err)
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("push oauth: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil || out.AccessToken == "" {
		return "", fmt.Errorf("push oauth: status %d", resp.StatusCode)
	}
	f.access, f.expiry = out.AccessToken, now.Add(time.Duration(out.ExpiresIn-60)*time.Second)
	return f.access, nil
}
