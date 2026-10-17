package media

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

// fakeS3 menerima PUT/DELETE object dan mencatatnya; fail = balas 403 (tidak di-retry SDK).
type fakeS3 struct {
	mu   sync.Mutex
	puts map[string]int
	dels []string
	fail bool
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		http.Error(w, `<Error><Code>AccessDenied</Code></Error>`, http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodPut:
		f.puts[r.URL.Path]++
	case http.MethodDelete:
		f.dels = append(f.dels, r.URL.Path)
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fakeS3) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.puts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestSyncToRemoteAndOrphanDelete(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	fake := &fakeS3{puts: map[string]int{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	root := t.TempDir()
	svc := NewService(NewRepository(pool), pmedia.NewDisk(root), pmedia.URLs{Base: "http://api.test"}).
		WithRemote(pmedia.NewS3(srv.URL, "key", "secret", "sisaguna-backup"))
	owner := fx.User("Mitra")

	img := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	pub, err := svc.Upload(ctx, owner, "listing", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	priv, err := svc.Upload(ctx, owner, "complaint", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	syncedAt := func(id string) (n int) {
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM media WHERE id=$1 AND r2_synced_at IS NOT NULL`, id).Scan(&n)
		return
	}

	// gagal -> tidak ditandai, dicoba lagi
	fake.fail = true
	if err := svc.SyncToRemote(ctx); err != nil {
		t.Fatal(err)
	}
	if syncedAt(pub.ID) != 0 {
		t.Fatal("upload gagal tidak boleh ditandai tersinkron")
	}

	fake.fail = false
	if err := svc.SyncToRemote(ctx); err != nil {
		t.Fatal(err)
	}
	if syncedAt(pub.ID) != 1 || syncedAt(priv.ID) != 1 {
		t.Fatal("kedua file harus tersinkron")
	}
	keys := fake.keys()
	var public, private, thumbs int
	for _, k := range keys {
		switch {
		case strings.HasPrefix(k, "/sisaguna-backup/media/public/"):
			public++
			if strings.HasSuffix(k, "_thumb.jpg") {
				thumbs++
			}
		case strings.HasPrefix(k, "/sisaguna-backup/media/private/"):
			private++
		default:
			t.Errorf("key tak terduga: %s", k)
		}
	}
	if public != 2 || thumbs != 1 || private != 1 {
		t.Errorf("key R2: %v", keys)
	}
	_ = svc.SyncToRemote(ctx) // sudah tersinkron: tidak ada upload ulang
	for k, n := range fake.puts {
		if n != 1 {
			t.Errorf("%s diupload %d kali", k, n)
		}
	}

	// file yatim > 24 jam dihapus dari disk, R2, dan tabel
	pool.Exec(ctx, `UPDATE media SET created_at = now() - interval '25 hours' WHERE id = $1`, uuid.MustParse(pub.ID))
	if err := svc.CleanupOrphans(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.dels) != 2 {
		t.Errorf("hapus di R2: %v", fake.dels)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM media WHERE id=$1`, pub.ID).Scan(&n)
	if n != 0 {
		t.Error("baris media yatim harus terhapus")
	}

	// R2 menolak hapus -> baris dan file dipertahankan untuk dicoba lagi
	pool.Exec(ctx, `UPDATE media SET created_at = now() - interval '25 hours' WHERE id = $1`, uuid.MustParse(priv.ID))
	fake.fail = true
	_ = svc.CleanupOrphans(ctx)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM media WHERE id=$1`, priv.ID).Scan(&n)
	if n != 1 {
		t.Error("kalau hapus di R2 gagal, baris tidak boleh dihapus")
	}
}
