package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{{Err: ErrMerchantNotFound, Status: http.StatusNotFound, Code: "merchant_not_found"}}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := api.Group("/admin", requireAuth, httpx.RequireAdmin())
	g.GET("/merchants", h.listMerchants)
	g.POST("/merchants/:id/approve", h.setStatus(sqlc.MerchantStatusApproved))
	g.POST("/merchants/:id/reject", h.setStatus(sqlc.MerchantStatusRejected))
}

type listQuery struct {
	httpx.PageQuery
	Status string `form:"status" binding:"omitempty,oneof=pending approved rejected suspended"`
}

func (h *Handler) listMerchants(c *gin.Context) {
	var q listQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	page, limit := q.Normalize()
	rows, total, err := h.svc.ListMerchants(c.Request.Context(), q.Status, page, limit)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OKMeta(c, rows, httpx.PageMeta(page, limit, total))
}

func (h *Handler) setStatus(status sqlc.MerchantStatus) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			httpx.Error(c, http.StatusNotFound, "merchant_not_found", ErrMerchantNotFound.Error())
			return
		}
		if err := h.svc.SetMerchantStatus(c.Request.Context(), id, status); err != nil {
			httpx.Fail(c, err, errRules)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
