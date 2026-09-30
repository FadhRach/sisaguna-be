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
		return writeUserServiceError(c, err)
	}
	return utils.WriteOK(c, user)
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
		return utils.WriteBadRequest(c, "payload update profil tidak valid")
	}

	user, err := h.userService.UpdateProfile(c.Context(), userID, services.UpdateProfileInput{
		Name:         req.Name,
		Phone:        req.Phone,
		BusinessName: req.BusinessName,
	})
	if err != nil {
		return writeUserServiceError(c, err)
	}
	return utils.WriteOK(c, user)
}

// GetByID menangani GET /users/:id, profil publik tanpa perlu login.
func (h *UserController) GetByID(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.WriteBadRequest(c, "id user tidak valid")
	}

	profile, err := h.userService.GetPublicProfile(c.Context(), userID)
	if err != nil {
		return writeUserServiceError(c, err)
	}
	return utils.WriteOK(c, profile)
}

func writeUserServiceError(c *fiber.Ctx, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.WriteNotFound(c, "user_not_found", "user tidak ditemukan")
	}
	return utils.WriteInternalError(c)
}
