package auth

import "net/http"

// Logout adalah entrypoint Vercel untuk POST /api/v1/auth/logout (butuh access token).
func Logout(w http.ResponseWriter, r *http.Request) {
	requireAuth(container.AuthHandler.Logout)(w, r)
}
