package utils

import "github.com/gofiber/fiber/v2"

// SuccessResponse adalah bentuk response sukses standar: data utama di "data",
// "meta" opsional untuk info pagination pada endpoint yang me-list banyak resource.
type SuccessResponse struct {
	Data interface{} `json:"data"`
	Meta *Meta       `json:"meta,omitempty"`
}

// Meta menampung info pagination. Field lain (filter/sort) sengaja tidak
// ditambahkan sebelum ada endpoint yang benar-benar mengisinya.
type Meta struct {
	Page      int   `json:"page"`
	Limit     int   `json:"limit"`
	Total     int64 `json:"total"`
	TotalPage int   `json:"total_page"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

// WriteJSON adalah primitif paling dasar: tulis status code + body JSON apa
// adanya. Helper Write* lain di bawah semua lewat sini — jarang dipanggil
// langsung dari controller.
func WriteJSON(c *fiber.Ctx, status int, data interface{}) error {
	if data == nil {
		return c.SendStatus(status)
	}
	return c.Status(status).JSON(data)
}

// WriteSuccess membungkus data ke {"data": ...} dengan status code custom.
func WriteSuccess(c *fiber.Ctx, status int, data interface{}) error {
	return WriteJSON(c, status, SuccessResponse{Data: data})
}

// WriteOK adalah WriteSuccess dengan status 200 — dipakai paling sering.
func WriteOK(c *fiber.Ctx, data interface{}) error {
	return WriteSuccess(c, fiber.StatusOK, data)
}

// WriteCreated dipakai setelah berhasil membuat resource baru (status 201).
func WriteCreated(c *fiber.Ctx, data interface{}) error {
	return WriteSuccess(c, fiber.StatusCreated, data)
}

// WriteNoContent dipakai untuk aksi berhasil tanpa body (mis. logout), status 204.
func WriteNoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// WriteSuccessPaginated membungkus list data + info pagination, status 200.
func WriteSuccessPaginated(c *fiber.Ctx, data interface{}, meta Meta) error {
	return WriteJSON(c, fiber.StatusOK, SuccessResponse{Data: data, Meta: &meta})
}

// WriteError menulis error sesuai format wajib di CLAUDE.md: {"error":{"code","message"}}.
func WriteError(c *fiber.Ctx, status int, code, message string) error {
	return WriteJSON(c, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

// Helper di bawah ini membungkus WriteError untuk kasus umum, supaya controller
// tidak perlu hardcode status+code berulang-ulang di tiap resource.

func WriteBadRequest(c *fiber.Ctx, message string) error {
	return WriteError(c, fiber.StatusBadRequest, "validation_error", message)
}

func WriteUnauthorized(c *fiber.Ctx, message string) error {
	return WriteError(c, fiber.StatusUnauthorized, "unauthorized", message)
}

func WriteForbidden(c *fiber.Ctx, message string) error {
	return WriteError(c, fiber.StatusForbidden, "forbidden", message)
}

func WriteNotFound(c *fiber.Ctx, code, message string) error {
	return WriteError(c, fiber.StatusNotFound, code, message)
}

func WriteConflict(c *fiber.Ctx, code, message string) error {
	return WriteError(c, fiber.StatusConflict, code, message)
}

func WriteInternalError(c *fiber.Ctx) error {
	return WriteError(c, fiber.StatusInternalServerError, "internal_error", "terjadi kesalahan internal")
}
