package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListContacts returns the agent's A2A contacts with tokens masked.
// @Summary      List contacts
// @Description  Contacts live in ~/.myaaw/<agent>/contacts.json and reference other A2A agents.
// @Tags         Contacts
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} ContactsResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/contacts [get]
func (h *Handler) ListContacts(c fiber.Ctx) error {
	contacts, err := h.contacts.List(c.Params("name"))
	if err != nil {
		return contactError(c, err)
	}
	return c.JSON(fiber.Map{"contacts": contacts})
}

// CreateContact registers another A2A agent as a contact.
// @Summary      Create a contact
// @Description  The token is the bearer token issued by the remote agent. It is stored in contacts.json and masked on output.
// @Tags         Contacts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body CreateContactRequest true "Contact definition"
// @Success      200 {object} service.ContactView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents/{name}/contacts [post]
func (h *Handler) CreateContact(c fiber.Ctx) error {
	var req CreateContactRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	view, err := h.contacts.Create(c.Params("name"), service.ContactInput{
		Name:        req.Name,
		URL:         req.URL,
		Token:       req.Token,
		Description: req.Description,
	})
	if err != nil {
		return contactError(c, err)
	}
	return c.JSON(view)
}

// UpdateContact edits a contact in place.
// @Summary      Update a contact
// @Description  Omitted fields keep their value. A non-empty token replaces the stored one; clear_token removes it.
// @Tags         Contacts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        contact path string true "Contact name"
// @Param        request body service.ContactUpdate true "Fields to change"
// @Success      200 {object} service.ContactView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents/{name}/contacts/{contact} [patch]
func (h *Handler) UpdateContact(c fiber.Ctx) error {
	var req service.ContactUpdate
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	view, err := h.contacts.Update(c.Params("name"), c.Params("contact"), req)
	if err != nil {
		return contactError(c, err)
	}
	return c.JSON(view)
}

// DeleteContact removes a contact.
// @Summary      Delete a contact
// @Tags         Contacts
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        contact path string true "Contact name"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/contacts/{contact} [delete]
func (h *Handler) DeleteContact(c fiber.Ctx) error {
	if err := h.contacts.Delete(c.Params("name"), c.Params("contact")); err != nil {
		return contactError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func contactError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrContactNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contact not found"})
	case errors.Is(err, service.ErrContactNameTaken):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidContact):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
