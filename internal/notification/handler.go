package notification

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{{Err: ErrNotFound, Status: http.StatusNotFound, Code: "notification_not_found"}}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := api.Group("/me/notifications", requireAuth)
	g.GET("", h.list)
	g.POST("/read-all", h.readAll)
	g.PATCH("/:id", h.markRead)
	g.DELETE("/:id", h.byID(h.svc.Delete))
	g.POST("/:id/restore", h.byID(h.svc.Restore))

	api.PUT("/me/devices", requireAuth, h.registerDevice)
	api.DELETE("/me/devices/:token", requireAuth, h.removeDevice)

	prefs := api.Group("/me/notification-prefs", requireAuth)
	prefs.GET("", h.prefs)
	prefs.PUT("", h.setPrefs)
}

func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	cur, ok := httpx.ParseCursor(c)
	if !ok {
		return
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	items, next, err := h.svc.List(c.Request.Context(), httpx.UserID(c), q.Type, cur, q.Limit)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OKMeta(c, items, httpx.Meta{"next_cursor": nilIfEmpty(next)})
}

func (h *Handler) markRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "notification_not_found", ErrNotFound.Error())
		return
	}
	var req MarkReadRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), httpx.UserID(c), id); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) readAll(c *gin.Context) {
	if err := h.svc.MarkAllRead(c.Request.Context(), httpx.UserID(c)); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

// byID membungkus aksi per notifikasi (hapus, pulihkan) yang menjawab 204.
func (h *Handler) byID(fn func(ctx context.Context, user, id uuid.UUID) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			httpx.Error(c, http.StatusNotFound, "notification_not_found", ErrNotFound.Error())
			return
		}
		if err := fn(c.Request.Context(), httpx.UserID(c), id); err != nil {
			httpx.Fail(c, err, errRules)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func (h *Handler) prefs(c *gin.Context) {
	out, err := h.svc.Prefs(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) setPrefs(c *gin.Context) {
	var req PrefsRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := h.svc.SetPrefs(c.Request.Context(), httpx.UserID(c), req)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) registerDevice(c *gin.Context) {
	var req DeviceRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.RegisterDevice(c.Request.Context(), httpx.UserID(c), req); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) removeDevice(c *gin.Context) {
	if err := h.svc.RemoveDevice(c.Request.Context(), httpx.UserID(c), c.Param("token")); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
