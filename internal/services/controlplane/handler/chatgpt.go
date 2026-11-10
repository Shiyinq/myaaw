package handler

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// localChatGPTRequest uses the socket peer and raw Host/Origin, never proxy
// headers or DNS resolution. This also blocks DNS rebinding and remote UIs
// whose browser would send the OAuth callback to the wrong machine.
func localChatGPTRequest(peer, host, origin string) bool {
	ip := net.ParseIP(peer)
	if ip == nil || !ip.IsLoopback() || !loopbackAuthority(host) {
		return false
	}
	if origin == "" {
		return true // Native same-machine clients do not send Origin.
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") &&
		u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && loopbackAuthority(u.Host)
}

func loopbackAuthority(authority string) bool {
	u, err := url.Parse("http://" + authority)
	if err != nil || u.User != nil || u.Host != authority || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLocalChatGPT(c fiber.Ctx) bool {
	// RemoteIP reads fasthttp's connection directly, independent of Fiber's
	// ProxyHeader/TrustProxy settings. Host is likewise read from the request.
	origin := c.Get("Origin")
	if !localChatGPTRequest(c.RequestCtx().RemoteIP().String(), string(c.Request().Host()), origin) {
		return false
	}
	// The frontend's same-origin proxy replaces Host but preserves these
	// browser headers. GET fetches may omit Origin, so also check Referer;
	// neither a remote dashboard nor a no-referrer browser may become local
	// merely because the proxy's upstream connection is loopback.
	referer := c.Get("Referer")
	if referer != "" {
		u, err := url.Parse(referer)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || !loopbackAuthority(u.Host) {
			return false
		}
	}
	if origin == "" && referer == "" && (c.Get("Sec-Fetch-Mode") != "" || c.Get("Sec-Fetch-Site") != "" || c.Get("Sec-Fetch-Dest") != "") {
		return false
	}
	return true
}

func rejectRemoteChatGPT(c fiber.Ctx) error {
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "ChatGPT Subscription browser sign-in requires a browser and control plane on the same machine. Open the dashboard through localhost, or run myaaw provider connect-chatgpt on the server; remote browser callbacks are not supported."})
}

func chatGPTHeaders(c fiber.Ctx) {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")
	c.Set("Referrer-Policy", "no-referrer")
}

func chatGPTError(c fiber.Ctx, err error) error {
	if errors.Is(err, service.ErrChatGPTSessionNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	if errors.Is(err, service.ErrChatGPTSessionLimit) {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": err.Error()})
	}
	return configWriteError(c, err)
}

type ChatGPTConnectRequest struct {
	CredentialID string `json:"credential_id,omitempty"`
}

// ConnectChatGPT starts an agent-scoped, same-machine browser sign-in.
// @Summary      Connect ChatGPT Subscription
// @Description  Requires loopback peer/host and local browser Origin/Referer, including through the frontend proxy. Authorization URLs are sensitive; never log them. The agent name may be new and is not persisted.
// @Tags         Providers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body ChatGPTConnectRequest true "Optional saved or pending credential ID"
// @Success      200 {object} service.ChatGPTConnectView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      429 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/chatgpt/connect [post]
func (h *Handler) ConnectChatGPT(c fiber.Ctx) error {
	chatGPTHeaders(c)
	if !isLocalChatGPT(c) {
		return rejectRemoteChatGPT(c)
	}
	var req ChatGPTConnectRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	view, err := h.config.StartChatGPT(c.Params("name"), strings.TrimSpace(req.CredentialID))
	if err != nil {
		return chatGPTError(c, err)
	}
	return c.JSON(view)
}

// ChatGPTStatus polls or cancels a scoped sign-in attempt.
// @Summary      ChatGPT Subscription sign-in status
// @Description  Requires same-machine access; browser GET requests without Origin need a local Referer. Status includes only pending/connected/failed/expired, credential_id, email, and sanitized error. Never includes tokens or authorization URLs.
// @Tags         Providers
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        session path string true "Sign-in session ID"
// @Success      200 {object} service.ChatGPTAttemptStatus
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/chatgpt/connect/{session} [get]
func (h *Handler) ChatGPTStatus(c fiber.Ctx) error {
	chatGPTHeaders(c)
	if !isLocalChatGPT(c) {
		return rejectRemoteChatGPT(c)
	}
	status, err := h.config.ChatGPTStatus(c.Params("name"), c.Params("session"), c.Method() == "DELETE")
	if err != nil {
		return chatGPTError(c, err)
	}
	if c.Method() == "DELETE" {
		return c.JSON(OKResponse{OK: true})
	}
	return c.JSON(status)
}

// CancelChatGPT cancels a same-machine sign-in attempt.
// @Summary      Cancel ChatGPT Subscription sign-in
// @Tags         Providers
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        session path string true "Sign-in session ID"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/chatgpt/connect/{session} [delete]
func (h *Handler) CancelChatGPT(c fiber.Ctx) error {
	return h.ChatGPTStatus(c)
}

// DisconnectProvider detaches a subscription provider reference only.
// @Summary      Disconnect ChatGPT Subscription provider
// @Description  Clears only this provider's credential reference. Does not delete shared host credentials or revoke the remote ChatGPT grant.
// @Tags         Providers
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Provider name"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/{id}/disconnect [post]
func (h *Handler) DisconnectProvider(c fiber.Ctx) error {
	chatGPTHeaders(c)
	if err := h.config.DisconnectProvider(c.Params("name"), c.Params("id")); err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(OKResponse{OK: true})
}
