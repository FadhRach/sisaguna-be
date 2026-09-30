// Package controllers adalah lapisan HTTP (Fiber): parse request, panggil
// service, tulis response lewat package utils. Satu file per resource (lihat
// Gaya kode di CLAUDE.md).
package controllers

import (
	"errors"
	"time"

	"github.com/FadhRach/sisaguna-be/services"
	"github.com/FadhRach/sisaguna-be/utils"
	"github.com/gofiber/fiber/v2"
)

type AuthController struct {
	authService services.AuthService
}

func NewAuthController(authService services.AuthService) *AuthController {
	return &AuthController{authService: authService}
}

type registerRequest struct {
	Name         string   `json:"name"`
	Email        string   `json:"email"`
	Phone        string   `json:"phone"`
	Password     string   `json:"password"`
	Roles        []string `json:"roles"`
	BusinessName *string  `json:"business_name,omitempty"`
}

func (h *AuthController) Register(c *fiber.Ctx) error {
	var req registerRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.WriteBadRequest(c, "payload register tidak valid")
	}

	if req.Name == "" || req.Email == "" || req.Phone == "" || req.Password == "" {
		return utils.WriteBadRequest(c, "name, email, phone, dan password wajib diisi")
	}

	user, err := h.authService.Register(c.Context(), services.RegisterInput{
		Name:         req.Name,
		Email:        req.Email,
		Phone:        req.Phone,
		Password:     req.Password,
		Roles:        req.Roles,
		BusinessName: req.BusinessName,
	})
	if err != nil {
		return writeAuthServiceError(c, err)
	}

	return utils.WriteCreated(c, user)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresAt    string      `json:"expires_at"`
	User         interface{} `json:"user"`
}

func (h *AuthController) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.WriteBadRequest(c, "payload login tidak valid")
	}

	if req.Email == "" || req.Password == "" {
		return utils.WriteBadRequest(c, "email dan password wajib diisi")
	}

	result, err := h.authService.Login(c.Context(), services.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		return writeAuthServiceError(c, err)
	}

	return utils.WriteOK(c, loginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    result.ExpiresAt.UTC().Format(time.RFC3339),
		User:         result.User,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresAt   string `json:"expires_at"`
}

func (h *AuthController) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil || req.RefreshToken == "" {
		return utils.WriteBadRequest(c, "refresh_token wajib diisi")
	}

	result, err := h.authService.Refresh(c.Context(), req.RefreshToken)
	if err != nil {
		return writeAuthServiceError(c, err)
	}

	return utils.WriteOK(c, refreshResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		ExpiresAt:   result.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout mengharuskan access token yang masih valid (lihat middleware.RequireAuth
// di routes/route.go) supaya refresh token yang direvoke pasti milik user yang login.
func (h *AuthController) Logout(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var req logoutRequest
	if err := c.BodyParser(&req); err != nil || req.RefreshToken == "" {
		return utils.WriteBadRequest(c, "refresh_token wajib diisi")
	}

	if err := h.authService.Logout(c.Context(), userID, req.RefreshToken); err != nil {
		return writeAuthServiceError(c, err)
	}

	return utils.WriteNoContent(c)
}

func writeAuthServiceError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, services.ErrEmailTaken):
		return utils.WriteConflict(c, "email_taken", err.Error())
	case errors.Is(err, services.ErrInvalidRole):
		return utils.WriteBadRequest(c, err.Error())
	case errors.Is(err, services.ErrInvalidCredentials):
		return utils.WriteError(c, fiber.StatusUnauthorized, "invalid_credentials", err.Error())
	case errors.Is(err, services.ErrInvalidRefreshToken):
		return utils.WriteError(c, fiber.StatusUnauthorized, "invalid_refresh_token", err.Error())
	default:
		return utils.WriteInternalError(c)
	}
}
