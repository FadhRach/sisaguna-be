// Package routes adalah satu-satunya sumber kebenaran untuk seluruh route
// aplikasi — semua endpoint didaftarkan di sini, tidak tersebar di file lain.
package routes

import (
	"github.com/FadhRach/sisaguna-be/controllers"
	"github.com/FadhRach/sisaguna-be/middleware"
	"github.com/gofiber/fiber/v2"
)

// Setup mendaftarkan semua route di bawah prefix /api/v1 (batasan kritis #4 di
// CLAUDE.md), satu function per resource biar gampang dibaca dan gampang
// nambah resource baru (listings, reservations, dst) nanti.
func Setup(app *fiber.App, authController *controllers.AuthController, userController *controllers.UserController, jwtSecret string) {
	requireAuth := middleware.RequireAuth(jwtSecret)
	api := app.Group("/api/v1")

	setupAuthRoutes(api, authController, requireAuth)
	setupUserRoutes(api, userController, requireAuth)
}

func setupAuthRoutes(router fiber.Router, controller *controllers.AuthController, requireAuth fiber.Handler) {
	auth := router.Group("/auth")
	auth.Post("/register", controller.Register)
	auth.Post("/login", controller.Login)
	auth.Post("/refresh", controller.Refresh)
	auth.Post("/logout", requireAuth, controller.Logout)
}

// Fiber otomatis resolve segmen statis ("/me") sebelum segmen dinamis
// ("/:id"), jadi "/users/:id" tidak akan "menelan" request ke "/users/me".
func setupUserRoutes(router fiber.Router, controller *controllers.UserController, requireAuth fiber.Handler) {
	users := router.Group("/users")
	users.Get("/me", requireAuth, controller.GetMe)
	users.Patch("/me", requireAuth, controller.UpdateMe)
	users.Get("/:id", controller.GetByID)
}
