package controllers

import (
	"errors"

	"github.com/FadhRach/sisaguna-be/services"
	"github.com/FadhRach/sisaguna-be/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserController struct {
	userService services.UserService
}

func NewUserController(userService services.UserService) *UserController {
	return &UserController{userService: userService}
}

// GetMe menangani GET /users/me.
func (h *UserController) GetMe(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	user, err := h.userService.GetProfile(c.Context(), userID)
	if err != nil {
		return writeUserServiceError(c, "Ambil profil", err)
	}
	return utils.Success(c, "Profil berhasil diambil", user)
}

type updateMeRequest struct {
	Name         *string `json:"name"`
	Phone        *string `json:"phone"`
	BusinessName *string `json:"business_name"`
}

// UpdateMe menangani PATCH /users/me.
func (h *UserController) UpdateMe(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var req updateMeRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Update profil gagal", "payload update profil tidak valid")
	}

	user, err := h.userService.UpdateProfile(c.Context(), userID, services.UpdateProfileInput{
		Name:         req.Name,
		Phone:        req.Phone,
		BusinessName: req.BusinessName,
	})
	if err != nil {
		return writeUserServiceError(c, "Update profil", err)
	}
	return utils.Success(c, "Profil berhasil diperbarui", user)
}

// GetByID menangani GET /users/:id, profil publik tanpa perlu login.
func (h *UserController) GetByID(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.BadRequest(c, "Ambil profil publik gagal", "id user tidak valid")
	}

	profile, err := h.userService.GetPublicProfile(c.Context(), userID)
	if err != nil {
		return writeUserServiceError(c, "Ambil profil publik", err)
	}
	return utils.Success(c, "Profil publik berhasil diambil", profile)
}

// writeUserServiceError memetakan error dari UserService ke response HTTP yang
// sesuai, pakai action buat message kontekstual (lihat writeAuthServiceError).
func writeUserServiceError(c *fiber.Ctx, action string, err error) error {
	message := action + " gagal"
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.NotFound(c, message, "user tidak ditemukan")
	}
	return utils.InternalServerError(c, message, err.Error())
}
