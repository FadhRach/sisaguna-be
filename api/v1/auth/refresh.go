package auth

import "net/http"

// Refresh adalah entrypoint Vercel untuk POST /api/v1/auth/refresh.
func Refresh(w http.ResponseWriter, r *http.Request) {
	container.AuthHandler.Refresh(w, r)
}
