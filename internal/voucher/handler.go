package voucher

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrUnavailable, Status: http.StatusNotFound, Code: "voucher_unavailable"},
	{Err: ErrAlreadyClaimed, Status: http.StatusConflict, Code: "voucher_already_claimed"},
	{Err: ErrInvalid, Status: http.StatusBadRequest, Code: "validation_failed"},
	{Err: ErrCodeTaken, Status: http.StatusConflict, Code: "voucher_code_taken"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "voucher_not_found"},
	{Err: ErrMerchantNotFound, Status: http.StatusNotFound, Code: "merchant_not_found"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	api.GET("/me/vouchers", requireAuth, h.list)
	api.POST("/vouchers/:code/claim", requireAuth, h.claim)

	admin := api.Group("/admin/vouchers", requireAuth, httpx.RequireAdmin())
	admin.GET("", h.adminList)
	admin.POST("", h.adminCreate)
	admin.PATCH("/:code", h.adminUpdate)
}

func (h *Handler) adminList(c *gin.Context) {
	var q AdminListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	page, limit := q.Normalize()
	rows, total, err := h.svc.AdminList(c.Request.Context(), page, limit)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OKMeta(c, rows, httpx.PageMeta(page, limit, total))
}

func (h *Handler) adminCreate(c *gin.Context) {
	var req AdminCreateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	h.respond(c, http.StatusCreated, req.Code, h.svc.Create(c.Request.Context(), req))
}

func (h *Handler) adminUpdate(c *gin.Context) {
	var req AdminUpdateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	h.respond(c, http.StatusOK, c.Param("code"), h.svc.Update(c.Request.Context(), c.Param("code"), req))
}

// respond mengembalikan voucher terbaru (bentuk admin) kalau aksi berhasil.
func (h *Handler) respond(c *gin.Context, status int, code string, err error) {
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), code)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, status, out)
}

func (h *Handler) list(c *gin.Context) {
	out, err := h.svc.List(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) claim(c *gin.Context) {
	if err := h.svc.Claim(c.Request.Context(), httpx.UserID(c), c.Param("code")); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}
