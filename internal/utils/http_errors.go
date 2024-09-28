package utils

import (
	"github.com/gofiber/fiber/v2"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func sendErrorResponse(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(ErrorResponse{Error: message})
}

func ErrorBadRequest(c *fiber.Ctx, message string) error {
	return sendErrorResponse(c, fiber.StatusBadRequest, message)
}

func ErrorUnauthorized(c *fiber.Ctx, message string) error {
	return sendErrorResponse(c, fiber.StatusUnauthorized, message)
}

func ErrorForbidden(c *fiber.Ctx, message string) error {
	return sendErrorResponse(c, fiber.StatusForbidden, message)
}

func ErrorNotFound(c *fiber.Ctx, message string) error {
	return sendErrorResponse(c, fiber.StatusNotFound, message)
}

func ErrorInternalServer(c *fiber.Ctx, message string) error {
	return sendErrorResponse(c, fiber.StatusInternalServerError, message)
}

func ErrorConflict(c *fiber.Ctx, message string) error {
	return sendErrorResponse(c, fiber.StatusConflict, message)
}

func ErrorValidation(c *fiber.Ctx, message string) error {
	return sendErrorResponse(c, fiber.StatusUnprocessableEntity, message)
}

func CustomError(c *fiber.Ctx, status int, message string) error {
	return sendErrorResponse(c, status, message)
}

func CustomErrorJSON(c *fiber.Ctx, status int, body interface{}) error {
	return c.Status(status).JSON(body)
}
