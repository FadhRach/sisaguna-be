package merchant

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrExists, Status: http.StatusConflict, Code: "merchant_exists"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "merchant_not_found"},
	{Err: ErrPartialPoint, Status: http.StatusBadRequest, Code: "validation_failed"},
	{Err: media.ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth, optionalAuth gin.HandlerFunc) {
	api.POST("/merchants", requireAuth, h.create)
	api.GET("/merchants", optionalAuth, h.list)
	api.GET("/merchants/:id", optionalAuth, h.public)
	api.GET("/me/merchant", requireAuth, h.mine)
	api.PATCH("/me/merchant", requireAuth, h.update)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := h.svc.Create(c.Request.Context(), httpx.UserID(c), req)
	h.respond(c, http.StatusCreated, out, err)
}

func (h *Handler) mine(c *gin.Context) {
	out, err := h.svc.Mine(c.Request.Context(), httpx.UserID(c))
	h.respond(c, http.StatusOK, out, err)
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := h.svc.Update(c.Request.Context(), httpx.UserID(c), req)
	h.respond(c, http.StatusOK, out, err)
}

func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	out, err := h.svc.List(c.Request.Context(), httpx.UserID(c), q)
	h.respond(c, http.StatusOK, out, err)
}

func (h *Handler) public(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "merchant_not_found", ErrNotFound.Error())
		return
	}
	var q PublicQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	out, err := h.svc.Public(c.Request.Context(), id, httpx.UserID(c), q)
	h.respond(c, http.StatusOK, out, err)
}

func (h *Handler) respond(c *gin.Context, status int, out any, err error) {
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, status, out)
}
