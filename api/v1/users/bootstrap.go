package users

import (
	"log"

	"github.com/FadhRach/sisaguna-be/internal/middleware"
	"github.com/FadhRach/sisaguna-be/internal/wiring"
)

// container dirakit sekali saat proses Vercel Function ini cold start.
var container = mustBuild()

// requireAuth dipakai berulang oleh entrypoint yang butuh login (mis. me.go).
var requireAuth = middleware.RequireAuth(container.Config.JWTSecret)

func mustBuild() *wiring.Container {
	c, err := wiring.Build()
	if err != nil {
		log.Fatalf("api/v1/users bootstrap: %v", err)
	}
	return c
}
