// Package handler adalah lapisan HTTP: decode request, panggil service,
// tulis response lewat internal/util. Satu file per resource (lihat Gaya
// kode di CLAUDE.md).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/FadhRach/sisaguna-be/internal/middleware"
	"github.com/FadhRach/sisaguna-be/internal/service"
	"github.com/FadhRach/sisaguna-be/internal/util"
	"github.com/google/uuid"
)

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type registerRequest struct {
	Name         string   `json:"name"`
	Email        string   `json:"email"`
	Phone        string   `json:"phone"`
	Password     string   `json:"password"`
	Roles        []string `json:"roles"`
	BusinessName *string  `json:"business_name,omitempty"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		util.WriteMethodNotAllowed(w)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteBadRequest(w, "payload register tidak valid")
		return
	}

	if req.Name == "" || req.Email == "" || req.Phone == "" || req.Password == "" {
		util.WriteBadRequest(w, "name, email, phone, dan password wajib diisi")
		return
	}

	user, err := h.authService.Register(r.Context(), service.RegisterInput{
		Name:         req.Name,
		Email:        req.Email,
		Phone:        req.Phone,
		Password:     req.Password,
		Roles:        req.Roles,
		BusinessName: req.BusinessName,
	})
	if err != nil {
		writeAuthServiceError(w, err)
		return
	}

	util.WriteCreated(w, user)
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

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		util.WriteMethodNotAllowed(w)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteBadRequest(w, "payload login tidak valid")
		return
	}

	if req.Email == "" || req.Password == "" {
		util.WriteBadRequest(w, "email dan password wajib diisi")
		return
	}

	result, err := h.authService.Login(r.Context(), service.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		writeAuthServiceError(w, err)
		return
	}

	util.WriteOK(w, loginResponse{
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

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		util.WriteMethodNotAllowed(w)
		return
	}

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		util.WriteBadRequest(w, "refresh_token wajib diisi")
		return
	}

	result, err := h.authService.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeAuthServiceError(w, err)
		return
	}

	util.WriteOK(w, refreshResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		ExpiresAt:   result.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout mengharuskan access token yang masih valid (lihat middleware.RequireAuth
// di entrypoint-nya) supaya refresh token yang direvoke pasti milik user yang login.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		util.WriteMethodNotAllowed(w)
		return
	}

	userID, ok := r.Context().Value(middleware.CtxUserID).(uuid.UUID)
	if !ok {
		util.WriteUnauthorized(w, "token tidak mengandung user id")
		return
	}

	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		util.WriteBadRequest(w, "refresh_token wajib diisi")
		return
	}

	if err := h.authService.Logout(r.Context(), userID, req.RefreshToken); err != nil {
		writeAuthServiceError(w, err)
		return
	}

	util.WriteNoContent(w)
}

func writeAuthServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrEmailTaken):
		util.WriteConflict(w, "email_taken", err.Error())
	case errors.Is(err, service.ErrInvalidRole):
		util.WriteBadRequest(w, err.Error())
	case errors.Is(err, service.ErrInvalidCredentials):
		util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", err.Error())
	case errors.Is(err, service.ErrInvalidRefreshToken):
		util.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", err.Error())
	default:
		util.WriteInternalError(w)
	}
}
