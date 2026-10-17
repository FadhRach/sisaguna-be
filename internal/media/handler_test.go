package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func newUser(t *testing.T, pool *pgxpool.Pool, tokens *auth.Tokens) (uuid.UUID, string) {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `INSERT INTO users (name,email,phone,password_hash) VALUES ('T',$1,'81234567890','x') RETURNING id`,
		uuid.NewString()+"@x.co").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, _ := tokens.Issue(id, false)
	return id, tok
}

func upload(t *testing.T, r http.Handler, token, purpose string, file []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if purpose != "" {
		_ = w.WriteField("purpose", purpose)
	}
	if file != nil {
		fw, _ := w.CreateFormFile("file", "foto.png")
		_, _ = fw.Write(file)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func get(r http.Handler, token, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) (data MediaDTO, code string) {
	t.Helper()
	var env struct {
		Data  MediaDTO `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return env.Data, env.Error.Code
}

func TestUploadEndpoints(t *testing.T) {
	pool := testutil.NewPool(t)
	root := t.TempDir()
	tokens := auth.NewTokens("test-secret-test-secret-test-secret-123", time.Minute)
	urls := pmedia.URLs{Base: "http://api.test"}
	h := NewHandler(NewService(NewRepository(pool), pmedia.NewDisk(root), urls))

	router, err := httpx.NewRouter(false, []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	h.Routes(router.Group("/api/v1"), httpx.RequireAuth(tokens), httpx.RateLimit(30, time.Hour, httpx.ByUser))

	_, owner := newUser(t, pool, tokens)
	_, stranger := newUser(t, pool, tokens)

	img := image.NewNRGBA(image.Rect(0, 0, 2400, 1200))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.White)
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)

	var first MediaDTO
	t.Run("public upload: resized, thumb, absolute urls, files on disk", func(t *testing.T) {
		rec := upload(t, router, owner, "avatar", pngBuf.Bytes())
		if rec.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		first, _ = decode(t, rec)
		if first.Width != 1600 || first.Height != 800 || first.ThumbURL == nil {
			t.Errorf("%+v", first)
		}
		if !strings.HasPrefix(first.URL, "http://api.test/media/") || strings.Contains(first.URL, root) {
			t.Errorf("url = %s", first.URL)
		}
		rel := strings.TrimPrefix(first.URL, "http://api.test/media/")
		if _, err := os.Stat(filepath.Join(root, "public", rel)); err != nil {
			t.Errorf("file missing: %v", err)
		}
	})

	t.Run("same file again returns the same media row", func(t *testing.T) {
		again, _ := decode(t, upload(t, router, owner, "avatar", pngBuf.Bytes()))
		if again.ID != first.ID {
			t.Errorf("ids differ: %s vs %s", again.ID, first.ID)
		}
	})

	var private MediaDTO
	t.Run("complaint photo is private: api url, no thumb, not under public/", func(t *testing.T) {
		private, _ = decode(t, upload(t, router, owner, "complaint", pngBuf.Bytes()))
		if !strings.HasSuffix(private.URL, "/api/v1/media/private/"+private.ID) || private.ThumbURL != nil {
			t.Errorf("%+v", private)
		}
		entries, _ := filepath.Glob(filepath.Join(root, "private", "*", "*.jpg"))
		if len(entries) != 1 {
			t.Errorf("private files: %v", entries)
		}
	})

	t.Run("private file: owner ok, stranger and anonymous denied", func(t *testing.T) {
		path := "/api/v1/media/private/" + private.ID
		if rec := get(router, owner, path); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
			t.Errorf("owner: %d %s", rec.Code, rec.Header().Get("Content-Type"))
		}
		if rec := get(router, stranger, path); rec.Code != http.StatusNotFound {
			t.Errorf("stranger: %d", rec.Code)
		}
		if rec := get(router, "", path); rec.Code != http.StatusUnauthorized {
			t.Errorf("anonymous: %d", rec.Code)
		}
		if rec := get(router, owner, "/api/v1/media/private/"+first.ID); rec.Code != http.StatusNotFound {
			t.Errorf("public media via private route: %d", rec.Code)
		}
	})

	t.Run("video: mp4 <= 30 detik untuk listing, metadata lokasi dibuang", func(t *testing.T) {
		rec := upload(t, router, owner, "listing", testMP4(5000))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		v, _ := decode(t, rec)
		if v.Kind != "video" || v.DurationMs == nil || *v.DurationMs != 5000 || v.ThumbURL != nil || !strings.HasSuffix(v.URL, ".mp4") {
			t.Fatalf("%+v", v)
		}
		onDisk, err := os.ReadFile(filepath.Join(root, "public", strings.TrimPrefix(v.URL, "http://api.test/media/")))
		if err != nil || bytes.Contains(onDisk, []byte("106.7989")) {
			t.Errorf("file video harus ada tanpa lokasi: %v", err)
		}
		if _, code := decode(t, upload(t, router, owner, "avatar", testMP4(5000))); code != "video_not_allowed" {
			t.Errorf("video untuk avatar: %q", code)
		}
		if rec := upload(t, router, owner, "listing", testMP4(31_000)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("video 31 detik: %d", rec.Code)
		}
	})

	t.Run("rejects bad requests", func(t *testing.T) {
		cases := []struct {
			name    string
			token   string
			purpose string
			file    []byte
			status  int
			code    string
		}{
			{"no auth", "", "avatar", pngBuf.Bytes(), 401, "unauthorized"},
			{"no purpose", owner, "", pngBuf.Bytes(), 400, "validation_failed"},
			{"bad purpose", owner, "banner", pngBuf.Bytes(), 400, "validation_failed"},
			{"no file", owner, "avatar", nil, 400, "file_required"},
			{"not an image", owner, "avatar", []byte("<html>halo</html>"), 415, "unsupported_media_type"},
			{"too large", owner, "avatar", bytes.Repeat([]byte{0xff}, pmedia.MaxImageSize+2048), 413, "file_too_large"},
		}
		for _, c := range cases {
			rec := upload(t, router, c.token, c.purpose, c.file)
			_, code := decode(t, rec)
			if rec.Code != c.status || code != c.code {
				t.Errorf("%s: got %d %q, want %d %q", c.name, rec.Code, code, c.status, c.code)
			}
		}
	})
}

// testMP4 membuat MP4 minimal: ftyp + moov(mvhd, udta berisi lokasi) + mdat.
func testMP4(durationMs uint32) []byte {
	box := func(name string, parts ...[]byte) []byte {
		body := bytes.Join(parts, nil)
		b := make([]byte, 8)
		binary.BigEndian.PutUint32(b, uint32(8+len(body)))
		copy(b[4:], name)
		return append(b, body...)
	}
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], durationMs)
	return bytes.Join([][]byte{
		box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41")),
		box("moov", box("mvhd", mvhd), box("udta", []byte("+06.2433+106.7989/"))),
		box("mdat", []byte("frame")),
	}, nil)
}
