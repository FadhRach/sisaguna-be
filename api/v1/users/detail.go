package users

import "net/http"

// GetUser adalah entrypoint Vercel untuk GET /api/v1/users/:id (profil publik, tanpa auth).
// Nama file sengaja bukan "[id].go" karena toolchain Go menolak kurung siku di
// nama file (lihat "go build" error); dynamic path-nya dipetakan lewat rewrite
// di vercel.json ke /api/v1/users/detail.
func GetUser(w http.ResponseWriter, r *http.Request) {
	container.UserHandler.GetByID(w, r)
}
