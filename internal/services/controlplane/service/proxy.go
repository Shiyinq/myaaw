package service

import (
	"bufio"
	"bytes"
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

func (p *ProxyService) Chat(c fiber.Ctx, agentName string) error {
	return p.forward(c, agentName, "/api/chat")
}

func (p *ProxyService) ChatStream(c fiber.Ctx, agentName string) error {
	return p.forward(c, agentName, "/api/chat/stream")
}

func (p *ProxyService) forward(c fiber.Ctx, agentName, path string) error {
	baseURL, _, err := p.agents.BaseURL(agentName)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, baseURL+path, bytes.NewReader(c.Body()))
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return c.Send(body)
}
