package address

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var errRules = []httpx.ErrorRule{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "address_not_found"},
	{Err: ErrLimit, Status: http.StatusUnprocessableEntity, Code: "address_limit"},
	{Err: ErrPartialPoint, Status: http.StatusBadRequest, Code: "validation_failed"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := api.Group("/me", requireAuth)
	g.GET("/addresses", h.list)
	g.POST("/addresses", h.create)
	g.PATCH("/addresses/:id", h.update)
	g.DELETE("/addresses/:id", h.delete)
	g.PUT("/selected-address", h.selectAddress)
}

func (h *Handler) list(c *gin.Context) {
	out, err := h.svc.List(c.Request.Context(), httpx.UserID(c))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := h.svc.Create(c.Request.Context(), httpx.UserID(c), req)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
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
	out, err := h.svc.Update(c.Request.Context(), id, httpx.UserID(c), req)
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
	if err := h.svc.Delete(c.Request.Context(), id, httpx.UserID(c)); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) selectAddress(c *gin.Context) {
	var req SelectRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Select(c.Request.Context(), httpx.UserID(c), req.AddressID); err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Status(http.StatusNoContent)
}

func paramID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "address_not_found", ErrNotFound.Error())
		return uuid.Nil, false
	}
	return id, true
}
