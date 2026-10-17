package media

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

var errRules = []httpx.ErrorRule{
	{Err: pmedia.ErrUnsupportedType, Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type"},
	{Err: pmedia.ErrImageTooLarge, Status: http.StatusUnprocessableEntity, Code: "image_too_large"},
	{Err: pmedia.ErrCorruptImage, Status: http.StatusUnprocessableEntity, Code: "corrupt_image"},
	{Err: pmedia.ErrVideoTooLong, Status: http.StatusUnprocessableEntity, Code: "video_too_long"},
	{Err: pmedia.ErrCorruptVideo, Status: http.StatusUnprocessableEntity, Code: "corrupt_video"},
	{Err: ErrVideoPurpose, Status: http.StatusBadRequest, Code: "video_not_allowed"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "media_not_found"},
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes: upload dibatasi 30 per jam per akun.
func (h *Handler) Routes(api *gin.RouterGroup, requireAuth gin.HandlerFunc, uploadLimit gin.HandlerFunc) {
	api.POST("/media", requireAuth, uploadLimit, h.upload)
	api.GET("/media/private/:id", requireAuth, h.private)
}

// upload menerima foto (JPEG/PNG/WebP, maks 8 MB) atau video MP4 (maks 30 MB, hanya purpose=listing).
// Jenisnya dikenali dari isi file, bukan dari nama atau Content-Type.
func (h *Handler) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, pmedia.MaxVideoSize+(1<<20)) // + overhead multipart
	var form UploadForm
	if !httpx.BindForm(c, &form) {
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpx.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file terlalu besar (foto maks 8 MB, video maks 30 MB)")
			return
		}
		httpx.Error(c, http.StatusBadRequest, "file_required", "field file wajib diisi")
		return
	}
	if fh.Size > pmedia.MaxVideoSize {
		httpx.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file terlalu besar (foto maks 8 MB, video maks 30 MB)")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, pmedia.MaxVideoSize+1))
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}

	var out *MediaDTO
	if pmedia.IsMP4(data) {
		out, err = h.svc.UploadVideoFor(c.Request.Context(), httpx.UserID(c), form.Purpose, data)
	} else if len(data) > pmedia.MaxImageSize {
		httpx.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "foto maksimal 8 MB")
		return
	} else {
		out, err = h.svc.Upload(c.Request.Context(), httpx.UserID(c), form.Purpose, data)
	}
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

func (h *Handler) private(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusNotFound, "media_not_found", ErrNotFound.Error())
		return
	}
	path, err := h.svc.PrivateFile(c.Request.Context(), httpx.UserID(c), httpx.IsAdmin(c), id)
	if err != nil {
		httpx.Fail(c, err, errRules)
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.File(path)
}
