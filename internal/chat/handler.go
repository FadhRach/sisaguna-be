package chat

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "conversation_not_found"},
	{Err: ErrMerchantNotFound, Status: http.StatusNotFound, Code: "merchant_not_found"},
	{Err: ErrOwnMerchant, Status: http.StatusForbidden, Code: "own_merchant"},
	{Err: ErrInvalidListing, Status: http.StatusBadRequest, Code: "invalid_listing"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes: sendLimit membatasi pengiriman pesan (20 per menit per akun).
func (h *Handler) Routes(api *gin.RouterGroup, requireAuth, sendLimit gin.HandlerFunc) {
	g := api.Group("/conversations", requireAuth)
	g.GET("", h.inbox)
	g.POST("", h.start)
	g.GET("/:id/messages", h.messages)
	g.POST("/:id/messages", sendLimit, h.send)
	g.POST("/:id/read", h.read)
}

func convID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "conversation_not_found", ErrNotFound.Error())
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) start(c *gin.Context) {
	var req StartRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := h.svc.Start(c.Request.Context(), httpx.UserID(c), req.MerchantID)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) inbox(c *gin.Context) {
	var q InboxQuery
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
	items, next, err := h.svc.Inbox(c.Request.Context(), httpx.UserID(c), q.As, cur, q.Limit)
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

func (h *Handler) messages(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	var q MessagesQuery
	if !httpx.BindForm(c, &q) {
		return
	}
	items, peerRead, err := h.svc.Messages(c.Request.Context(), httpx.UserID(c), id, q)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OKMeta(c, items, httpx.Meta{"peer_last_read_id": peerRead})
}

func (h *Handler) send(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	var req SendRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	msg, created, err := h.svc.Send(c.Request.Context(), httpx.UserID(c), id, req)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	status := http.StatusOK // kirim ulang dengan client_id yang sama
	if created {
		status = http.StatusCreated
	}
	httpx.OK(c, status, msg)
}

func (h *Handler) read(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	var req ReadRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	last, err := h.svc.MarkRead(c.Request.Context(), httpx.UserID(c), id, *req.LastReadID)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, map[string]int64{"last_read_id": last})
}
