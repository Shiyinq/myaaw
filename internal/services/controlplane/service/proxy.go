package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gofiber/fiber/v3"
)

// ProxyService forwards chat requests from the control plane to the selected
// agent server. Streaming responses are relayed chunk by chunk.
type ProxyService struct {
	agents *AgentService
	client *http.Client
}

func NewProxyService(agents *AgentService) *ProxyService {
	return &ProxyService{
		agents: agents,
		client: &http.Client{},
	}
}

func (p *ProxyService) Chat(c fiber.Ctx, agentName string, userID int) error {
	return p.forward(c, agentName, "/api/chat", userID)
}

func (p *ProxyService) ChatStream(c fiber.Ctx, agentName string, userID int) error {
	return p.forward(c, agentName, "/api/chat/stream", userID)
}

func (p *ProxyService) forward(c fiber.Ctx, agentName, path string, userID int) error {
	baseURL, _, err := p.agents.BaseURL(agentName)
	if err != nil {
		return err
	}

	body := injectUserID(c.Body(), userID)
	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if accept := c.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	if lastEventID := c.Get("Last-Event-ID"); lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}

	c.Status(resp.StatusCode)
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Set("Content-Type", contentType)
	}
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		c.Set("Cache-Control", cacheControl)
	}

	if path == "/api/chat/stream" {
		c.Set("Cache-Control", "no-cache")
		// The body must stay open until the stream writer finishes; closing it
		// here would empty the SSE response.
		return c.SendStreamWriter(func(w *bufio.Writer) {
			defer resp.Body.Close()
			buf := make([]byte, 32*1024)
			for {
				n, readErr := resp.Body.Read(buf)
				if n > 0 {
					if _, writeErr := w.Write(buf[:n]); writeErr != nil {
						return
					}
					if flushErr := w.Flush(); flushErr != nil {
						return
					}
				}
				if readErr != nil {
					return
				}
			}
		})
	}

	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return c.Send(responseBody)
}

// injectUserID always sets user_id from the authenticated control-plane user.
// The control plane represents one logged-in user, so a user_id in the request
// body is ignored and cannot escape the configured conversation scope.
func injectUserID(body []byte, userID int) []byte {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		// Leave malformed JSON untouched; the agent server returns the error.
		return body
	}

	encoded, err := json.Marshal(userID)
	if err != nil {
		return body
	}
	payload["user_id"] = encoded

	updated, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return updated
}
