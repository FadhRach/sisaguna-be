package review

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
	{Err: ErrWindowClosed, Status: http.StatusConflict, Code: "review_window_closed"},
	{Err: ErrAlreadyReviewed, Status: http.StatusConflict, Code: "already_reviewed"},
	{Err: ErrMerchantMissing, Status: http.StatusNotFound, Code: "merchant_not_found"},
	{Err: media.ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	api.POST("/orders/:id/review", requireAuth, h.create)
	api.GET("/merchants/:id/reviews", h.list)
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

func (h *Handler) list(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "merchant_not_found", ErrMerchantMissing.Error())
		return
	}
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
	items, next, err := h.svc.ListByMerchant(c.Request.Context(), id, cur, q.Limit)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	var nextPtr *string
	if next != "" {
		nextPtr = &next
	}
	httpx.OKMeta(c, items, httpx.Meta{"next_cursor": nextPtr})
}
