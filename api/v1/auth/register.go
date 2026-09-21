package auth

import "net/http"

// Register adalah entrypoint Vercel untuk POST /api/v1/auth/register.
func Register(w http.ResponseWriter, r *http.Request) {
	container.AuthHandler.Register(w, r)
}
