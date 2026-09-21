// Package wiring merakit seluruh dependency (config -> db -> repository ->
// service -> handler) sekali, dipakai bareng oleh cmd/main.go dan tiap
// entrypoint /api supaya wiring-nya tidak copy-paste di banyak tempat.
package wiring

import (
	"fmt"

	"github.com/FadhRach/sisaguna-be/internal/config"
	"github.com/FadhRach/sisaguna-be/internal/db"
	"github.com/FadhRach/sisaguna-be/internal/handler"
	"github.com/FadhRach/sisaguna-be/internal/repository"
	"github.com/FadhRach/sisaguna-be/internal/service"
)

// Container membungkus seluruh dependency yang sudah dirakit, dipakai bareng
// oleh cmd/main.go (local dev) dan tiap entrypoint /api (Vercel) — supaya
// wiring cuma ditulis sekali, tidak copy-paste di tiap file route.
type Container struct {
	Config      *config.Config
	AuthHandler *handler.AuthHandler
	UserHandler *handler.UserHandler
}

func Build() (*Container, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("wiring build: %w", err)
	}

	gormDB, err := db.Get(cfg)
	if err != nil {
		return nil, fmt.Errorf("wiring build: %w", err)
	}

	userRepo := repository.NewUserRepository(gormDB)
	refreshTokenRepo := repository.NewRefreshTokenRepository(gormDB)
	authService := service.NewAuthService(userRepo, refreshTokenRepo, cfg.JWTSecret, cfg.JWTAccessTokenTTL, cfg.JWTRefreshTokenTTL)
	userService := service.NewUserService(userRepo)

	return &Container{
		Config:      cfg,
		AuthHandler: handler.NewAuthHandler(authService),
		UserHandler: handler.NewUserHandler(userService),
	}, nil
}
