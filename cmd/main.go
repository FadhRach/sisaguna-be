package main

import (
	"log"
	"net/http"

	"github.com/FadhRach/sisaguna-be/internal/middleware"
	"github.com/FadhRach/sisaguna-be/internal/wiring"
)

func main() {
	container, err := wiring.Build()
	if err != nil {
		log.Fatalf("main: %v", err)
	}

	requireAuth := middleware.RequireAuth(container.Config.JWTSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", container.AuthHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", container.AuthHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", container.AuthHandler.Refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", requireAuth(container.AuthHandler.Logout))
	mux.HandleFunc("GET /api/v1/users/me", requireAuth(container.UserHandler.Me))
	mux.HandleFunc("PATCH /api/v1/users/me", requireAuth(container.UserHandler.Me))
	mux.HandleFunc("GET /api/v1/users/{id}", container.UserHandler.GetByID)

	addr := ":" + container.Config.Port
	log.Printf("SisaGuna backend jalan di %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
