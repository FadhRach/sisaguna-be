package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func httpDetect(b []byte) string {
	mime := http.DetectContentType(b)
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	return mime
}

type Visibility string

const (
	Public  Visibility = "public"
	Private Visibility = "private"
)

// Disk menyimpan file di MEDIA_ROOT/{public|private}/<2 char>/<sha256>.jpg.
type Disk struct{ root string }

func NewDisk(root string) *Disk { return &Disk{root: root} }

// Paths menurunkan path relatif (terhadap folder visibility) dari hash isi.
// File privat diberi prefix pada hash supaya tidak pernah bertabrakan dengan file publik yang sama persis.
func Paths(v Visibility, content []byte) (sha, rel, thumbRel string) {
	sha = Hash(v, content)
	return sha, sha[:2] + "/" + sha + ".jpg", sha[:2] + "/" + sha + "_thumb.jpg"
}

// VideoPath adalah path relatif video dengan hash yang sama dengan Hash.
func VideoPath(sha string) string { return sha[:2] + "/" + sha + ".mp4" }

// Hash adalah SHA-256 isi file; file privat diberi prefix supaya tidak sama dengan file publik.
func Hash(v Visibility, content []byte) string {
	h := sha256.New()
	if v == Private {
		h.Write([]byte("private:"))
	}
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// Write menulis atomik (file sementara lalu rename) supaya pembaca tidak pernah melihat file setengah jadi.
func (d *Disk) Write(v Visibility, rel string, data []byte) error {
	dst := d.path(v, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("media mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".upload-*")
	if err != nil {
		return fmt.Errorf("media temp file: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op setelah rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("media write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("media close: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("media chmod: %w", err)
	}
	return os.Rename(tmp.Name(), dst)
}

// FilePath mengembalikan path disk untuk disajikan; path dari DB, tidak pernah dari input user.
func (d *Disk) FilePath(v Visibility, rel string) string { return d.path(v, rel) }

func (d *Disk) path(v Visibility, rel string) string {
	return filepath.Join(d.root, string(v), filepath.FromSlash(rel))
}

// Writable dipakai /healthz: folder media bisa ditulis.
func (d *Disk) Writable() error {
	if err := os.MkdirAll(d.root, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(d.root, ".health-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

// URLs menyusun URL absolut; path disk tidak pernah dikembalikan ke client.
type URLs struct{ Base string }

func (u URLs) Public(rel string) string { return u.Base + "/media/" + rel }

func (u URLs) PublicPtr(rel *string) *string {
	if rel == nil || *rel == "" {
		return nil
	}
	s := u.Public(*rel)
	return &s
}

func (u URLs) Private(id string) string { return u.Base + "/api/v1/media/private/" + id }

// Remove menghapus file; file yang memang sudah tidak ada dianggap berhasil.
func (d *Disk) Remove(v Visibility, rel string) error {
	if err := os.Remove(d.path(v, rel)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
