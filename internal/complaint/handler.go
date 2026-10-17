package complaint

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrOrderNotFound, Status: http.StatusNotFound, Code: "order_not_found"},
	{Err: ErrNotCompleted, Status: http.StatusConflict, Code: "order_not_completed"},
	{Err: ErrWindowClosed, Status: http.StatusConflict, Code: "complaint_window_closed"},
	{Err: ErrAlreadyComplaint, Status: http.StatusConflict, Code: "already_complained"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "complaint_not_found"},
	{Err: ErrClosed, Status: http.StatusConflict, Code: "complaint_closed"},
	{Err: ErrNoteRequired, Status: http.StatusBadRequest, Code: "validation_failed"},
	{Err: media.ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	api.POST("/orders/:id/complaint", requireAuth, h.create)
	admin := api.Group("/admin/complaints", requireAuth, httpx.RequireAdmin())
	admin.GET("", h.adminList)
	admin.PATCH("/:id", h.adminUpdate)
}

func (h *Handler) create(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "order_not_found", ErrOrderNotFound.Error())
		return
	}
	var req CreateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := h.svc.Create(c.Request.Context(), httpx.UserID(c), id, req)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

func (h *Handler) adminList(c *gin.Context) {
	var q AdminListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	page, limit := q.Normalize()
	rows, total, err := h.svc.AdminList(c.Request.Context(), q.Status, page, limit)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OKMeta(c, rows, httpx.PageMeta(page, limit, total))
}

func (h *Handler) adminUpdate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "complaint_not_found", ErrNotFound.Error())
		return
	}
	var req UpdateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, req); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}
