package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrEmailTaken, Status: http.StatusConflict, Code: "email_taken"},
	{Err: ErrInvalidCredentials, Status: http.StatusUnauthorized, Code: "invalid_credentials"},
	{Err: ErrInvalidRefreshToken, Status: http.StatusUnauthorized, Code: "invalid_refresh_token"},
	{Err: ErrUserNotFound, Status: http.StatusNotFound, Code: "user_not_found"},
	{Err: ErrWrongPassword, Status: http.StatusBadRequest, Code: "wrong_password"},
	{Err: ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
}

type Handler struct {
	svc  *Service
	urls pmedia.URLs
}

func NewHandler(svc *Service, urls pmedia.URLs) *Handler { return &Handler{svc: svc, urls: urls} }

// Limits adalah rate limit per rute; dibuat di main.go supaya state-nya tidak global.
type Limits struct{ Login, Register, Password gin.HandlerFunc }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc, l Limits) {
	a := api.Group("/auth")
	a.POST("/register", l.Register, h.register)
	a.POST("/login", l.Login, h.login)
	a.POST("/refresh", h.refresh)
	a.POST("/logout", requireAuth, h.logout)
	a.POST("/password", requireAuth, l.Password, h.changePassword)
	api.GET("/me", requireAuth, h.me)
	api.PATCH("/me", requireAuth, h.updateMe)
	api.GET("/me/impact", requireAuth, h.impact)
}

func (h *Handler) register(c *gin.Context) {
	var req RegisterRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	sess, err := h.svc.Register(c.Request.Context(), req, c.Request.UserAgent())
	h.respondSession(c, http.StatusCreated, sess, err)
}

func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	sess, err := h.svc.Login(c.Request.Context(), req, c.Request.UserAgent())
	h.respondSession(c, http.StatusOK, sess, err)
}

func (h *Handler) refresh(c *gin.Context) {
	var req RefreshRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	sess, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, c.Request.UserAgent())
	h.respondSession(c, http.StatusOK, sess, err)
}

func (h *Handler) changePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	sess, err := h.svc.ChangePassword(c.Request.Context(), httpx.UserID(c), req.Current, req.New, c.Request.UserAgent())
	h.respondSession(c, http.StatusOK, sess, err)
}

func (h *Handler) logout(c *gin.Context) {
	var req RefreshRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Logout(c.Request.Context(), httpx.UserID(c), req.RefreshToken); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) me(c *gin.Context) {
	p, err := h.svc.Me(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, MeDTO{ProfileDTO: h.profileDTO(p.GetProfileRow), UnreadNotifications: p.UnreadNotifications})
}

func (h *Handler) impact(c *gin.Context) {
	out, err := h.svc.Impact(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) updateMe(c *gin.Context) {
	var req UpdateProfileRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.UpdateProfile(c.Request.Context(), httpx.UserID(c), req); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	h.me(c)
}

func (h *Handler) respondSession(c *gin.Context, status int, s *Session, err error) {
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, status, SessionDTO{AccessToken: s.AccessToken, RefreshToken: s.RefreshToken, ExpiresAt: s.ExpiresAt.UTC(), User: h.profileDTO(s.User)})
}

func (h *Handler) profileDTO(u sqlc.GetProfileRow) ProfileDTO {
	p := ProfileDTO{ID: u.ID.String(), Name: u.Name, Email: u.Email, Phone: u.Phone, IsAdmin: u.IsAdmin, CreatedAt: u.CreatedAt.Time.UTC()}
	if u.AvatarKey.Valid {
		p.AvatarKey = &u.AvatarKey.String
	}
	if u.LocationLabel.Valid {
		p.LocationLabel = &u.LocationLabel.String
	}
	if u.SelectedAddressID.Valid {
		s := u.SelectedAddressID.UUID.String()
		p.SelectedAddressID = &s
	}
	if u.PhotoPath.Valid {
		p.PhotoURL = h.urls.PublicPtr(&u.PhotoPath.String)
	}
	return p
}
