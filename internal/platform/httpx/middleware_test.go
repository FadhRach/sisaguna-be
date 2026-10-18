package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
)

func TestRequireAuthLimitedOnlyCountsWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := auth.NewTokens("test-secret-test-secret-test-secret-123", time.Minute)
	userA, _, _ := tokens.Issue(uuid.New(), false)
	userB, _, _ := tokens.Issue(uuid.New(), false)

	r := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	g := r.Group("", RequireAuthLimited(tokens, 3, nil))
	g.GET("/read", ok)
	g.POST("/write", ok)
	g.DELETE("/other-write", ok)

	call := func(method, path, token string) int {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := call("POST", "/write", ""); code != 401 {
		t.Errorf("tanpa token: %d", code)
	}
	for range 20 { // GET tidak dihitung
		if code := call("GET", "/read", userA); code != 204 {
			t.Fatalf("GET: %d", code)
		}
	}
	for i := range 3 { // burst 3, lintas endpoint tulis berbagi satu ember
		method, path := "POST", "/write"
		if i == 2 {
			method, path = "DELETE", "/other-write"
		}
		if code := call(method, path, userA); code != 204 {
			t.Fatalf("tulis ke-%d: %d", i+1, code)
		}
	}
	if code := call("POST", "/write", userA); code != 429 {
		t.Errorf("tulis ke-4 harus 429, dapat %d", code)
	}
	if code := call("GET", "/read", userA); code != 204 {
		t.Errorf("GET tetap boleh saat tulis terkunci: %d", code)
	}
	if code := call("POST", "/write", userB); code != 204 {
		t.Errorf("akun lain tidak terpengaruh: %d", code)
	}
}

func TestRequireAuthChecksDatabaseState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := auth.NewTokens("test-secret-test-secret-test-secret-123", time.Minute)
	alive, deleted, demoted, revoked := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	state := map[uuid.UUID]AuthState{alive: {}, demoted: {}, revoked: {ValidAfter: time.Now().Add(time.Hour)}}
	checker := NewSessionChecker(func(_ context.Context, id uuid.UUID) (AuthState, bool, error) {
		st, ok := state[id]
		return st, ok, nil
	}, time.Hour)

	r := gin.New()
	g := r.Group("", RequireAuthLimited(tokens, 100, checker))
	g.GET("/me", func(c *gin.Context) { c.JSON(200, gin.H{"admin": IsAdmin(c)}) })

	call := func(id uuid.UUID, tokenAdmin bool) (int, string) {
		tok, _, _ := tokens.Issue(id, tokenAdmin)
		req := httptest.NewRequest("GET", "/me", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	if code, _ := call(alive, false); code != 200 {
		t.Errorf("akun aktif: %d", code)
	}
	if code, _ := call(deleted, false); code != 401 {
		t.Errorf("akun terhapus harus 401: %d", code)
	}
	if code, _ := call(revoked, false); code != 401 {
		t.Errorf("sesi dicabut harus 401: %d", code)
	}
	if code, body := call(demoted, true); code != 200 || body != `{"admin":false}` {
		t.Errorf("klaim admin di token tidak boleh dipercaya: %d %s", code, body)
	}
}
