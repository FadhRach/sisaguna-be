// Package middleware berisi Fiber middleware yang dipasang per-route di
// routes/route.go (bukan chain global), supaya jelas endpoint mana saja yang
// butuh login.
package middleware

import (
	"strings"

	"github.com/FadhRach/sisaguna-be/utils"
	"github.com/gofiber/fiber/v2"
)

const (
	LocalsKeyUserID = "user_id"
	LocalsKeyEmail  = "email"
	LocalsKeyRoles  = "roles"
)

// RequireAuth memvalidasi JWT dari header Authorization dan menaruh klaimnya
// di c.Locals, siap dibaca controller lewat LocalsKeyUserID dst.
func RequireAuth(jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, ok := strings.CutPrefix(c.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			return utils.WriteUnauthorized(c, "header Authorization Bearer token wajib diisi")
		}

		claims, err := utils.ParseToken(jwtSecret, token)
		if err != nil {
			return utils.WriteUnauthorized(c, "token tidak valid atau kedaluwarsa")
		}

		c.Locals(LocalsKeyUserID, claims.UserID)
		c.Locals(LocalsKeyEmail, claims.Email)
		c.Locals(LocalsKeyRoles, claims.Roles)
		return c.Next()
	}
}
