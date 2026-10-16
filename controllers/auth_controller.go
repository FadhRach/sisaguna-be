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
		return utils.BadRequest(c, "Registrasi gagal", "payload register tidak valid")
	}

	if req.Name == "" || req.Email == "" || req.Phone == "" || req.Password == "" {
		return utils.BadRequest(c, "Registrasi gagal", "name, email, phone, dan password wajib diisi")
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
		return writeAuthServiceError(c, "Registrasi", err)
	}

	return utils.Created(c, "Registrasi berhasil", user)
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
		return utils.BadRequest(c, "Login gagal", "payload login tidak valid")
	}

	if req.Email == "" || req.Password == "" {
		return utils.BadRequest(c, "Login gagal", "email dan password wajib diisi")
	}

	result, err := h.authService.Login(c.Context(), services.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		return writeAuthServiceError(c, "Login", err)
	}

	return utils.Success(c, "Login berhasil", loginResponse{
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
		return utils.BadRequest(c, "Refresh token gagal", "refresh_token wajib diisi")
	}

	result, err := h.authService.Refresh(c.Context(), req.RefreshToken)
	if err != nil {
		return writeAuthServiceError(c, "Refresh token", err)
	}

	return utils.Success(c, "Refresh token berhasil", refreshResponse{
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
		return utils.BadRequest(c, "Logout gagal", "refresh_token wajib diisi")
	}

	if err := h.authService.Logout(c.Context(), userID, req.RefreshToken); err != nil {
		return writeAuthServiceError(c, "Logout", err)
	}

	return utils.NoContent(c)
}

// writeAuthServiceError memetakan error dari AuthService ke response HTTP yang
// sesuai. action dipakai buat bikin message kontekstual ("Registrasi gagal",
// "Login gagal", dst) tanpa nulis ulang switch-nya di tiap method.
func writeAuthServiceError(c *fiber.Ctx, action string, err error) error {
	message := action + " gagal"
	switch {
	case errors.Is(err, services.ErrEmailTaken):
		return utils.Conflict(c, message, err.Error())
	case errors.Is(err, services.ErrInvalidRole):
		return utils.BadRequest(c, message, err.Error())
	case errors.Is(err, services.ErrInvalidCredentials), errors.Is(err, services.ErrInvalidRefreshToken):
		return utils.Unauthorized(c, message, err.Error())
	default:
		return utils.InternalServerError(c, message, err.Error())
	}
}
