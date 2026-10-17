package saved

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{{Err: ErrMerchantNotFound, Status: http.StatusNotFound, Code: "merchant_not_found"}}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := api.Group("/me/saved-merchants", requireAuth)
	g.GET("", h.list)
	g.PUT("/:merchantId", h.change(h.svc.Save))
	g.DELETE("/:merchantId", h.change(h.svc.Remove))
}

func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	out, err := h.svc.List(c.Request.Context(), httpx.UserID(c), q)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// change membungkus Save/Remove: keduanya idempoten dan menjawab 204.
func (h *Handler) change(fn func(ctx context.Context, user, merchant uuid.UUID) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("merchantId"))
		if err != nil {
			httpx.Error(c, http.StatusNotFound, "merchant_not_found", ErrMerchantNotFound.Error())
			return
		}
		if err := fn(c.Request.Context(), httpx.UserID(c), id); err != nil {
			httpx.Fail(c, err, errRules)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
