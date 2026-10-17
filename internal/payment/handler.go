package payment

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrInvalidPhone, Status: http.StatusBadRequest, Code: "validation_failed"},
	{Err: ErrFixed, Status: http.StatusUnprocessableEntity, Code: "payment_method_fixed"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "payment_method_not_found"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := api.Group("/me/payment-methods", requireAuth)
	g.GET("", func(c *gin.Context) { h.respond(c, http.StatusOK)(h.svc.List(c.Request.Context(), httpx.UserID(c))) })
	g.POST("", func(c *gin.Context) {
		var req LinkRequest
		if !httpx.BindJSON(c, &req) {
			return
		}
		h.respond(c, http.StatusCreated)(h.svc.Link(c.Request.Context(), httpx.UserID(c), req))
	})
	g.PUT("/default", func(c *gin.Context) {
		var req DefaultRequest
		if !httpx.BindJSON(c, &req) {
			return
		}
		h.respond(c, http.StatusOK)(h.svc.SetDefault(c.Request.Context(), httpx.UserID(c), req.Kind))
	})
	g.DELETE("/:kind", func(c *gin.Context) {
		h.respond(c, http.StatusOK)(h.svc.Unlink(c.Request.Context(), httpx.UserID(c), c.Param("kind")))
	})
}

// respond mengembalikan daftar metode terbaru, jadi app cukup mengganti state-nya dari satu respons.
func (h *Handler) respond(c *gin.Context, status int) func([]MethodDTO, error) {
	return func(out []MethodDTO, err error) {
		if err != nil {
			httpx.Fail(c, err, errRules)
			return
		}
		httpx.OK(c, status, out)
	}
}
