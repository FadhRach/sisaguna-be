package listing

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "listing_not_found"},
	{Err: ErrMerchantRequired, Status: http.StatusForbidden, Code: "merchant_required"},
	{Err: ErrMerchantNotApproved, Status: http.StatusForbidden, Code: "merchant_not_approved"},
	{Err: ErrInvalidPrice, Status: http.StatusUnprocessableEntity, Code: "invalid_price"},
	{Err: ErrMadeAtRequired, Status: http.StatusUnprocessableEntity, Code: "made_at_required"},
	{Err: ErrChecklistIncomplete, Status: http.StatusUnprocessableEntity, Code: "checklist_incomplete"},
	{Err: ErrPastSafeWindow, Status: http.StatusUnprocessableEntity, Code: "past_safe_window"},
	{Err: ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth, optionalAuth gin.HandlerFunc) {
	api.GET("/home", optionalAuth, h.home)
	api.GET("/listings", h.list)
	api.GET("/listings/:id", h.get)
	api.POST("/listings", requireAuth, h.create)
	api.PATCH("/listings/:id", requireAuth, h.update)
	api.DELETE("/listings/:id", requireAuth, h.delete)
	api.GET("/me/merchant/listings", requireAuth, h.mine)
}

func (h *Handler) home(c *gin.Context) {
	var q HomeQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	out, err := h.svc.Home(c.Request.Context(), q, httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	out, total, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	page, limit := q.Normalize()
	httpx.OKMeta(c, out, httpx.PageMeta(page, limit, total))
}

func (h *Handler) get(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var q PointQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id, q)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) create(c *gin.Context) {
	key, ok := httpx.IdempotencyKey(c)
	if !ok {
		return
	}
	var req CreateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, created, err := h.svc.Create(c.Request.Context(), httpx.UserID(c), key, req)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id, PointQuery{})
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	httpx.OK(c, status, out)
}

func (h *Handler) update(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Update(c.Request.Context(), httpx.UserID(c), id, req); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id, PointQuery{})
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) delete(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), httpx.UserID(c), id); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) mine(c *gin.Context) {
	var q httpx.PageQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	page, limit := q.Normalize()
	out, total, err := h.svc.Mine(c.Request.Context(), httpx.UserID(c), page, limit)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OKMeta(c, out, httpx.PageMeta(page, limit, total))
}

func paramID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "listing_not_found", ErrNotFound.Error())
		return uuid.Nil, false
	}
	return id, true
}
