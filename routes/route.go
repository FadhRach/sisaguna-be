// Package routes adalah satu-satunya sumber kebenaran untuk seluruh route
// aplikasi — semua endpoint didaftarkan di sini, tidak tersebar di file lain.
package routes

import (
	"github.com/FadhRach/sisaguna-be/controllers"
	"github.com/FadhRach/sisaguna-be/middleware"
	"github.com/gofiber/fiber/v2"
)

// Setup mendaftarkan semua route di bawah prefix /api/v1 (batasan kritis #4 di
// CLAUDE.md). Fiber otomatis resolve segmen statis ("/me") sebelum segmen
// dinamis ("/:id"), jadi tidak perlu rewrite khusus untuk disambiguasi.
func Setup(app *fiber.App, authController *controllers.AuthController, userController *controllers.UserController, jwtSecret string) {
	requireAuth := middleware.RequireAuth(jwtSecret)

	api := app.Group("/api/v1")

	auth := api.Group("/auth")
	auth.Post("/register", authController.Register)
	auth.Post("/login", authController.Login)
	auth.Post("/refresh", authController.Refresh)
	auth.Post("/logout", requireAuth, authController.Logout)

	users := api.Group("/users")
	users.Get("/me", requireAuth, userController.GetMe)
	users.Patch("/me", requireAuth, userController.UpdateMe)
	users.Get("/:id", userController.GetByID)
}
