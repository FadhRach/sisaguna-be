package order

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrDeliveryUnavailable, Status: http.StatusUnprocessableEntity, Code: "delivery_unavailable"},
	{Err: ErrPaymentUnavailable, Status: http.StatusUnprocessableEntity, Code: "payment_method_unavailable"},
	{Err: ErrPaymentNotLinked, Status: http.StatusUnprocessableEntity, Code: "payment_method_not_linked"},
	{Err: ErrAddressNotFound, Status: http.StatusNotFound, Code: "address_not_found"},
	{Err: ErrMerchantUnavailable, Status: http.StatusNotFound, Code: "merchant_unavailable"},
	{Err: ErrMerchantRequired, Status: http.StatusForbidden, Code: "merchant_required"},
	{Err: ErrOwnMerchant, Status: http.StatusForbidden, Code: "own_merchant"},
	{Err: ErrInvalidCart, Status: http.StatusBadRequest, Code: "invalid_cart"},
	{Err: ErrListingUnavailable, Status: http.StatusConflict, Code: "listing_unavailable"},
	{Err: ErrOutOfStock, Status: http.StatusConflict, Code: "out_of_stock"},
	{Err: ErrVoucherNotClaimed, Status: http.StatusUnprocessableEntity, Code: "voucher_not_claimed"},
	{Err: ErrVoucherUsed, Status: http.StatusUnprocessableEntity, Code: "voucher_used"},
	{Err: ErrVoucherInvalid, Status: http.StatusUnprocessableEntity, Code: "voucher_invalid"},
	{Err: ErrVoucherMinSpend, Status: http.StatusUnprocessableEntity, Code: "voucher_min_spend"},
	{Err: ErrPriceChanged, Status: http.StatusConflict, Code: "price_changed"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "order_not_found"},
	{Err: ErrInvalidState, Status: http.StatusConflict, Code: "invalid_order_state"},
	{Err: ErrCancelWindowClosed, Status: http.StatusConflict, Code: "cancel_window_closed"},
	{Err: ErrWrongCode, Status: http.StatusUnprocessableEntity, Code: "wrong_code"},
	{Err: ErrTooManyAttempts, Status: http.StatusTooManyRequests, Code: "too_many_attempts"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := api.Group("", requireAuth)
	g.POST("/checkout/quote", h.quote)
	g.POST("/orders", h.place)
	g.GET("/orders/:id", h.get)
	g.POST("/orders/:id/cancel", h.cancelByBuyer)
	g.GET("/me/orders", h.listBuyer)

	g.GET("/me/merchant/dashboard", h.dashboard)
	g.GET("/me/merchant/orders", h.listMerchant)
	g.POST("/me/merchant/orders/:id/confirm", h.confirm)
	g.POST("/me/merchant/orders/:id/cancel", h.cancelByMerchant)
}

func (h *Handler) quote(c *gin.Context) {
	var req QuoteRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := h.svc.Quote(c.Request.Context(), httpx.UserID(c), req)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) place(c *gin.Context) {
	key, ok := httpx.IdempotencyKey(c)
	if !ok {
		return
	}
	var req PlaceRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, created, err := h.svc.Place(c.Request.Context(), httpx.UserID(c), key, req)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), httpx.UserID(c), id)
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

func (h *Handler) get(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), httpx.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) cancelByBuyer(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var req CancelRequest
	if c.Request.ContentLength != 0 && !httpx.BindJSON(c, &req) { // body opsional
		return
	}
	if err := h.svc.CancelByBuyer(c.Request.Context(), httpx.UserID(c), id, req.Reason); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	h.respondOrder(c, id)
}

func (h *Handler) cancelByMerchant(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var req MerchantCancelRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.CancelByMerchant(c.Request.Context(), httpx.UserID(c), id, req.Reason); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	h.respondOrder(c, id)
}

func (h *Handler) confirm(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var req ConfirmRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Confirm(c.Request.Context(), httpx.UserID(c), id, req.Code); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	h.respondOrder(c, id)
}

func (h *Handler) respondOrder(c *gin.Context, id uuid.UUID) {
	out, err := h.svc.Get(c.Request.Context(), httpx.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) listBuyer(c *gin.Context) {
	var q BuyerListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	cur, ok := httpx.ParseCursor(c)
	if !ok {
		return
	}
	out, next, err := h.svc.ListBuyer(c.Request.Context(), httpx.UserID(c), q, cur)
	h.respondList(c, out, next, err)
}

func (h *Handler) listMerchant(c *gin.Context) {
	var q MerchantListQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	cur, ok := httpx.ParseCursor(c)
	if !ok {
		return
	}
	out, next, err := h.svc.ListMerchant(c.Request.Context(), httpx.UserID(c), q, cur)
	h.respondList(c, out, next, err)
}

func (h *Handler) respondList(c *gin.Context, out []OrderDTO, next string, err error) {
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	var nc *string
	if next != "" {
		nc = &next
	}
	httpx.OKMeta(c, out, httpx.Meta{"next_cursor": nc})
}

func (h *Handler) dashboard(c *gin.Context) {
	out, err := h.svc.Dashboard(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func paramID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "order_not_found", ErrNotFound.Error())
		return uuid.Nil, false
	}
	return id, true
}
