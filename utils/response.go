package utils

import "github.com/gofiber/fiber/v2"

// Response adalah bentuk response standar, niru pola reference project:
// {"status","response_code","message","data","error"}.
type Response struct {
	Status       string      `json:"status"`
	ResponseCode int         `json:"response_code"`
	Message      string      `json:"message,omitempty"`
	Data         interface{} `json:"data,omitempty"`
	Error        string      `json:"error,omitempty"`
}

type ResponsePaginated struct {
	Status       string         `json:"status"`
	ResponseCode int            `json:"response_code"`
	Message      string         `json:"message,omitempty"`
	Data         interface{}    `json:"data,omitempty"`
	Meta         PaginationMeta `json:"meta"`
}

type PaginationMeta struct {
	Page      int `json:"page"`
	Limit     int `json:"limit"`
	Total     int `json:"total"`
	TotalPage int `json:"total_pages"`
}

func Success(c *fiber.Ctx, message string, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(Response{Status: "Success", ResponseCode: fiber.StatusOK, Message: message, Data: data})
}

func SuccessPagination(c *fiber.Ctx, message string, data interface{}, meta PaginationMeta) error {
	return c.Status(fiber.StatusOK).JSON(ResponsePaginated{Status: "Success", ResponseCode: fiber.StatusOK, Message: message, Data: data, Meta: meta})
}

func Created(c *fiber.Ctx, message string, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(Response{Status: "Created", ResponseCode: fiber.StatusCreated, Message: message, Data: data})
}

func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func BadRequest(c *fiber.Ctx, message string, err string) error {
	return c.Status(fiber.StatusBadRequest).JSON(Response{Status: "Error Bad Request", ResponseCode: fiber.StatusBadRequest, Message: message, Error: err})
}

func Unauthorized(c *fiber.Ctx, message string, err string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(Response{Status: "Error Unauthorized", ResponseCode: fiber.StatusUnauthorized, Message: message, Error: err})
}

func Conflict(c *fiber.Ctx, message string, err string) error {
	return c.Status(fiber.StatusConflict).JSON(Response{Status: "Error Conflict", ResponseCode: fiber.StatusConflict, Message: message, Error: err})
}

func NotFound(c *fiber.Ctx, message string, err string) error {
	return c.Status(fiber.StatusNotFound).JSON(Response{Status: "Error Not Found", ResponseCode: fiber.StatusNotFound, Message: message, Error: err})
}

func InternalServerError(c *fiber.Ctx, message string, err string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(Response{Status: "Error Internal Server", ResponseCode: fiber.StatusInternalServerError, Message: message, Error: err})
}
