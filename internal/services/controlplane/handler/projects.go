package handler

import (
	"errors"
	"net/url"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ProjectStatus classifies a project and returns its preview state.
// @Summary      Project preview status
// @Description  Reports the detected type, size, build state, and a signed preview URL for a project directory.
// @Tags         Projects
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Virtual project path" default(/home/workspace/site)
// @Success      200 {object} service.ProjectStatus
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/project/status [get]
func (h *Handler) ProjectStatus(c fiber.Ctx) error {
	name := c.Params("name")
	status, err := h.projects.Status(name, c.Query("path"))
	if err != nil {
		return projectError(c, err)
	}
	h.attachPreviewURL(name, &status)
	return c.JSON(status)
}

// BuildProject starts a sandboxed build for a built project.
// @Summary      Build a project preview
// @Description  Copies the project to a temp directory, installs dependencies, runs the build, and caches the output for 15 minutes.
// @Tags         Projects
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Virtual project path"
// @Success      202 {object} service.ProjectStatus
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/project/build [post]
func (h *Handler) BuildProject(c fiber.Ctx) error {
	name := c.Params("name")
	status, _, err := h.projects.StartBuild(name, c.Query("path"))
	if err != nil {
		return projectError(c, err)
	}
	h.attachPreviewURL(name, &status)
	return c.Status(fiber.StatusAccepted).JSON(status)
}

// ProjectPreview serves one file from a project preview. The signed token lives
// in the path so relative subresources keep working inside the iframe.
// @Summary      Serve a project preview file
// @Tags         Projects
// @Produce      application/octet-stream
// @Param        name path string true "Agent name"
// @Param        token path string true "Signed preview token"
// @Success      200 {file} file
// @Success      206 {file} file "Partial content"
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/project/preview/{token} [get]
func (h *Handler) ProjectPreview(c fiber.Ctx) error {
	name := c.Params("name")
	token := c.Params("token")
	virtualPath, ok := h.auth.ValidateProjectPreview(name, token)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired preview signature"})
	}

	abs, err := h.projects.PreviewFile(name, virtualPath, c.Params("*"))
	if err != nil {
		return projectError(c, err)
	}

	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cache-Control", "no-store")
	// The iframe is sandboxed without allow-same-origin, so this document has an
	// opaque (null) origin. Module scripts and crossorigin stylesheets are then
	// cross-origin requests even though the URL is on this host, which needs CORS.
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Cross-Origin-Resource-Policy", "cross-origin")
	if strings.HasSuffix(strings.ToLower(abs), ".html") || strings.HasSuffix(strings.ToLower(abs), ".htm") {
		// The iframe is sandboxed by the dashboard as well; this keeps a directly
		// opened preview from acquiring the control-plane origin's privileges.
		//
		// 'self' cannot be used here: in an opaque origin it matches no URL. The
		// scheme sources let the document load its own scripts, styles, images,
		// and the webfonts a project links, while connect-src 'none' still blocks
		// fetch/XHR/WebSocket exfiltration.
		c.Set("Content-Security-Policy",
			"sandbox allow-scripts allow-forms allow-modals allow-popups; "+
				"default-src 'none'; "+
				"script-src http: https: 'unsafe-inline' 'unsafe-eval'; "+
				"style-src http: https: 'unsafe-inline'; "+
				"img-src http: https: data: blob:; "+
				"font-src http: https: data:; "+
				"media-src http: https: data: blob:; "+
				"worker-src http: https: blob:; "+
				"connect-src 'none'; "+
				"frame-src 'none'; "+
				"form-action 'none'")
	}
	c.Set("Content-Type", service.ContentTypeForPreview(abs))
	return c.SendFile(abs, fiber.SendFile{ByteRange: true, CacheDuration: -1})
}

// attachPreviewURL signs a ready project's preview root and points the UI at
// its entry file.
func (h *Handler) attachPreviewURL(agent string, status *service.ProjectStatus) {
	if status.State != "ready" {
		return
	}
	token, exp, err := h.auth.SignProjectPreview(agent, status.Path)
	if err != nil {
		return
	}
	entry := status.Entry
	if entry == "" {
		entry = "index.html"
	}
	segments := strings.Split(entry, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	status.URL = "/agents/" + url.PathEscape(agent) + "/project/preview/" + token + "/" + strings.Join(segments, "/")
	status.ExpiresAt = exp
}

func projectError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound), errors.Is(err, service.ErrPathNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrProjectNotDirectory), errors.Is(err, service.ErrProjectNotBuildable):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrProjectNotReady), errors.Is(err, service.ErrProjectBuilding):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidPath), errors.Is(err, service.ErrRootProtected):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
