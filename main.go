package main

import (
	"log"

	"github.com/FadhRach/sisaguna-be/config"
	"github.com/FadhRach/sisaguna-be/controllers"
	"github.com/FadhRach/sisaguna-be/repositories"
	"github.com/FadhRach/sisaguna-be/routes"
	"github.com/FadhRach/sisaguna-be/services"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("main: %v", err)
	}

	if _, err := config.Connect(cfg); err != nil {
		log.Fatalf("main: %v", err)
	}

	userRepo := repositories.NewUserRepository()
	refreshTokenRepo := repositories.NewRefreshTokenRepository()
	authService := services.NewAuthService(userRepo, refreshTokenRepo, cfg.JWTSecret, cfg.JWTAccessTokenTTL, cfg.JWTRefreshTokenTTL)
	userService := services.NewUserService(userRepo)
	authController := controllers.NewAuthController(authService)
	userController := controllers.NewUserController(userService)

	app := fiber.New()
	// Proses ini long-lived dan dipakai bareng semua request (beda dari dulu
	// yang satu proses Vercel Function per request) — tanpa recover(), satu
	// panic di mana pun bisa jatuhin seluruh server, bukan cuma satu request.
	app.Use(recover.New())
	routes.Setup(app, authController, userController, cfg.JWTSecret)

	log.Printf("SisaGuna backend jalan di :%s", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}
