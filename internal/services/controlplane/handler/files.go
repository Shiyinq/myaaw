package handler

import (
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

func (h *Handler) resolveProjectRoot(c fiber.Ctx, convID string) (string, error) {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return "", nil
	}
	return h.conversations.ResolveWorkingDir(c.Params("name"), convID, h.requestUserID(c))
}

// ListFiles browses an agent directory.
// @Summary      Browse agent files
// @Description  Lists the virtual roots (/home, /user, /skills, /jobs, plus /project when conversation_id has an attached working directory) or the entries of one directory.
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string false "Virtual path" default(/)
// @Param        conversation_id query string false "Conversation ID to scope /project virtual root"
// @Success      200 {object} FileListResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs [get]
func (h *Handler) ListFiles(c fiber.Ctx) error {
	path := c.Query("path")
	projectRoot, err := h.resolveProjectRoot(c, c.Query("conversation_id"))
	if err != nil {
		return conversationError(c, err)
	}
	entries, err := h.files.ListInProject(c.Params("name"), path, projectRoot)
	if err != nil {
		return fileError(c, err)
	}
	if path == "" {
		path = "/"
	}
	return c.JSON(fiber.Map{"path": path, "entries": entries})
}

// ReadFile returns a text file.
// @Summary      Read an agent file
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Virtual path" default(/home/AGENTS.md)
// @Param        conversation_id query string false "Conversation ID to scope /project virtual root"
// @Success      200 {object} service.FileContent
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Failure      415 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/read [get]
func (h *Handler) ReadFile(c fiber.Ctx) error {
	projectRoot, err := h.resolveProjectRoot(c, c.Query("conversation_id"))
	if err != nil {
		return conversationError(c, err)
	}
	content, err := h.files.ReadInProject(c.Params("name"), c.Query("path"), projectRoot)
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(content)
}

// WriteFile writes a text file.
// @Summary      Write an agent file
// @Tags         Files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body WriteFileRequest true "Path and content"
// @Success      200 {object} service.FileContent
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/write [post]
func (h *Handler) WriteFile(c fiber.Ctx) error {
	var req WriteFileRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	convID := strings.TrimSpace(req.ConversationID)
	if convID == "" {
		convID = c.Query("conversation_id")
	}
	projectRoot, err := h.resolveProjectRoot(c, convID)
	if err != nil {
		return conversationError(c, err)
	}
	content, err := h.files.WriteInProject(c.Params("name"), req.Path, req.Content, projectRoot)
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(content)
}

// Mkdir creates a directory.
// @Summary      Create an agent directory
// @Tags         Files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body MkdirRequest true "Directory path"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/mkdir [post]
func (h *Handler) Mkdir(c fiber.Ctx) error {
	var req MkdirRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	convID := strings.TrimSpace(req.ConversationID)
	if convID == "" {
		convID = c.Query("conversation_id")
	}
	projectRoot, err := h.resolveProjectRoot(c, convID)
	if err != nil {
		return conversationError(c, err)
	}
	if err := h.files.MkdirInProject(c.Params("name"), req.Path, projectRoot); err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// RenameFile moves a file or directory.
// @Summary      Rename or move an agent path
// @Tags         Files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body RenameRequest true "Source and destination"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/rename [post]
func (h *Handler) RenameFile(c fiber.Ctx) error {
	var req RenameRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	convID := strings.TrimSpace(req.ConversationID)
	if convID == "" {
		convID = c.Query("conversation_id")
	}
	projectRoot, err := h.resolveProjectRoot(c, convID)
	if err != nil {
		return conversationError(c, err)
	}
	if err := h.files.RenameInProject(c.Params("name"), req.From, req.To, projectRoot); err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// DeleteFile removes a file or directory.
// @Summary      Delete an agent path
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Virtual path"
// @Param        recursive query bool false "Delete directories recursively"
// @Param        conversation_id query string false "Conversation ID to scope /project virtual root"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs [delete]
func (h *Handler) DeleteFile(c fiber.Ctx) error {
	recursive := false
	if raw := c.Query("recursive"); raw != "" {
		recursive, _ = strconv.ParseBool(raw)
	}
	projectRoot, err := h.resolveProjectRoot(c, c.Query("conversation_id"))
	if err != nil {
		return conversationError(c, err)
	}
	if err := h.files.DeleteInProject(c.Params("name"), c.Query("path"), recursive, projectRoot); err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// SearchFiles searches text under an agent root.
// @Summary      Search agent files
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        q query string true "Search text"
// @Param        path query string false "Virtual path" default(/home)
// @Param        conversation_id query string false "Conversation ID to scope /project virtual root"
// @Success      200 {object} SearchResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/search [get]
func (h *Handler) SearchFiles(c fiber.Ctx) error {
	projectRoot, err := h.resolveProjectRoot(c, c.Query("conversation_id"))
	if err != nil {
		return conversationError(c, err)
	}
	matches, err := h.files.SearchInProject(c.Params("name"), c.Query("path"), c.Query("q"), projectRoot)
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"matches": matches})
}

// SignFiles returns short-lived signatures for one page of media paths, so
// <img>/<video> URLs do not need the session token.
// @Summary      Sign media paths
// @Tags         Files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body SignFilesRequest true "Paths to sign"
// @Success      200 {object} SignFilesResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      503 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/sign [post]
func (h *Handler) SignFiles(c fiber.Ctx) error {
	var req SignFilesRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if len(req.Paths) == 0 || len(req.Paths) > maxSignPaths {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "paths must contain 1-200 entries"})
	}
	convID := strings.TrimSpace(req.ConversationID)
	if convID == "" {
		convID = strings.TrimSpace(c.Query("conversation_id"))
	}
	scopeKey := c.Params("name")
	if convID != "" {
		if _, err := h.resolveProjectRoot(c, convID); err != nil {
			return conversationError(c, err)
		}
		scopeKey = scopeKey + ":" + convID
	}
	exp, urls, err := h.auth.SignMedia(scopeKey, req.Paths)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	if convID != "" {
		q := "&conversation_id=" + url.QueryEscape(convID)
		for p, suffix := range urls {
			urls[p] = suffix + q
		}
	}
	return c.JSON(SignFilesResponse{ExpiresAt: exp, URLs: urls})
}

const maxSignPaths = 200

type SignFilesRequest struct {
	Paths          []string `json:"paths"`
	ConversationID string   `json:"conversation_id,omitempty"`
}

type SignFilesResponse struct {
	ExpiresAt int64             `json:"expires_at"`
	URLs      map[string]string `json:"urls"`
}

// RawFile streams a file with its real content type, so the browser can render
// images, video, audio, and PDFs inline. Range requests are enabled, which is
// what lets a <video> element seek without downloading the whole file.
// @Summary      Stream an agent file
// @Description  Serves the raw bytes with the extension's content type and range support. Pass download=1 to force a save dialog.
// @Tags         Files
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Virtual path" default(/home/AGENTS.md)
// @Param        download query bool false "Send Content-Disposition: attachment"
// @Param        conversation_id query string false "Conversation ID to scope /project virtual root"
// @Success      200 {file} file
// @Success      206 {file} file "Partial content"
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/raw [get]
func (h *Handler) RawFile(c fiber.Ctx) error {
	projectRoot, err := h.resolveProjectRoot(c, c.Query("conversation_id"))
	if err != nil {
		return conversationError(c, err)
	}
	abs, _, err := h.files.RawInProject(c.Params("name"), c.Query("path"), projectRoot)
	if err != nil {
		return fileError(c, err)
	}

	// Never let a browser guess a type from the bytes: an HTML or SVG file that
	// is opened directly must not acquire the control-plane origin's privileges.
	c.Set("X-Content-Type-Options", "nosniff")
	switch strings.ToLower(filepath.Ext(abs)) {
	case ".html", ".htm", ".svg", ".xml":
		c.Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:")
	}

	download := c.Query("download") == "1" || strings.EqualFold(c.Query("download"), "true")
	return c.SendFile(abs, fiber.SendFile{
		ByteRange:     true,
		Download:      download,
		CacheDuration: -1,
	})
}

func fileError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrPathNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "path not found"})
	case errors.Is(err, service.ErrPathTooLarge):
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrBinaryFile):
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidPath), errors.Is(err, service.ErrRootProtected):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
