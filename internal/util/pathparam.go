package util

import "net/http"

// PathParam membaca parameter path dari route stdlib (Go 1.22+ pola {name})
// lalu fallback ke query string. Vercel Go runtime belum mendokumentasikan
// resmi cara dynamic route ([id].go) meneruskan segmen path, jadi dua sumber
// ini dibaca supaya handler tetap jalan di kedua environment (lihat CLAUDE.md).
func PathParam(r *http.Request, name string) string {
	if value := r.PathValue(name); value != "" {
		return value
	}
	return r.URL.Query().Get(name)
}
