package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/FadhRach/sisaguna-be/internal/middleware"
	"github.com/FadhRach/sisaguna-be/internal/service"
	"github.com/FadhRach/sisaguna-be/internal/util"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserHandler struct {
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// Me menangani GET dan PATCH /users/me sekaligus, karena satu file Vercel = satu route.
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.CtxUserID).(uuid.UUID)
	if !ok {
		util.WriteUnauthorized(w, "token tidak mengandung user id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getMe(w, r, userID)
	case http.MethodPatch:
		h.updateMe(w, r, userID)
	default:
		util.WriteMethodNotAllowed(w)
	}
}

func (h *UserHandler) getMe(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	user, err := h.userService.GetProfile(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err)
		return
	}
	util.WriteOK(w, user)
}

type updateMeRequest struct {
	Name         *string `json:"name"`
	Phone        *string `json:"phone"`
	BusinessName *string `json:"business_name"`
}

func (h *UserHandler) updateMe(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteBadRequest(w, "payload update profil tidak valid")
		return
	}

	user, err := h.userService.UpdateProfile(r.Context(), userID, service.UpdateProfileInput{
		Name:         req.Name,
		Phone:        req.Phone,
		BusinessName: req.BusinessName,
	})
	if err != nil {
		writeUserServiceError(w, err)
		return
	}
	util.WriteOK(w, user)
}

// GetByID menangani GET /users/:id, profil publik tanpa perlu login.
func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		util.WriteMethodNotAllowed(w)
		return
	}

	idParam := util.PathParam(r, "id")
	userID, err := uuid.Parse(idParam)
	if err != nil {
		util.WriteBadRequest(w, "id user tidak valid")
		return
	}

	profile, err := h.userService.GetPublicProfile(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err)
		return
	}
	util.WriteOK(w, profile)
}

func writeUserServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		util.WriteNotFound(w, "user_not_found", "user tidak ditemukan")
		return
	}
	util.WriteInternalError(w)
}
