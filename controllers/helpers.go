package controllers

import (
	"github.com/FadhRach/sisaguna-be/middleware"
	"github.com/FadhRach/sisaguna-be/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// requireUserID membaca user id yang ditaruh middleware.RequireAuth di
// c.Locals. Dipakai berulang oleh controller manapun yang butuh identitas
// user yang login (bukan inline copy-paste di tiap method).
func requireUserID(c *fiber.Ctx) (uuid.UUID, error) {
	userID, ok := c.Locals(middleware.LocalsKeyUserID).(uuid.UUID)
	if !ok {
		return uuid.Nil, utils.Unauthorized(c, "Autentikasi gagal", "token tidak mengandung user id")
	}
	return userID, nil
}
