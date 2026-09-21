// Package middleware berisi wrapper yang dipanggil eksplisit per entrypoint
// (bukan middleware chain terpusat) — lihat catatan kenapa di RequireAuth.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/FadhRach/sisaguna-be/internal/util"
)

type contextKey string

const (
	CtxUserID contextKey = "user_id"
	CtxEmail  contextKey = "email"
	CtxRoles  contextKey = "roles"
)

// RequireAuth membungkus handler dengan validasi JWT dari header Authorization.
// Dipanggil eksplisit per entrypoint (bukan lewat middleware chain terpusat),
// karena tiap file di /api adalah Vercel Function/proses terpisah.
func RequireAuth(jwtSecret string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				util.WriteError(w, http.StatusUnauthorized, "unauthorized", "header Authorization Bearer token wajib diisi")
				return
			}

			claims, err := util.ParseToken(jwtSecret, token)
			if err != nil {
				util.WriteError(w, http.StatusUnauthorized, "unauthorized", "token tidak valid atau kedaluwarsa")
				return
			}

			ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
			ctx = context.WithValue(ctx, CtxEmail, claims.Email)
			ctx = context.WithValue(ctx, CtxRoles, claims.Roles)
			next(w, r.WithContext(ctx))
		}
	}
}
