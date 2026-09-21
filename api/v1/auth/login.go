package auth

import "net/http"

// Login adalah entrypoint Vercel untuk POST /api/v1/auth/login.
func Login(w http.ResponseWriter, r *http.Request) {
	container.AuthHandler.Login(w, r)
}
