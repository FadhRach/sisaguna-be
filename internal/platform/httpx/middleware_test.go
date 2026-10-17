package httpx

import (
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
	g := r.Group("", RequireAuthLimited(tokens, 3))
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
