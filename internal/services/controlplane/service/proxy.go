package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/agent/approvals"

	"github.com/gofiber/fiber/v3"
)

// ProxyService forwards chat requests from the control plane to the selected
// agent server. Streaming responses are relayed chunk by chunk.
type ProxyService struct {
	agents *AgentService
	groups *GroupService
	client *http.Client
}

func NewProxyService(agents *AgentService) *ProxyService {
	return &ProxyService{
		agents: agents,
		client: &http.Client{},
	}
}

// SetGroups wires GroupService into the multiplexed GET /events stream so
// group status and message previews stay updated across all dashboard routes.
func (p *ProxyService) SetGroups(groups *GroupService) {
	p.groups = groups
}

func (p *ProxyService) Chat(c fiber.Ctx, agentName string, userID int) error {
	return p.forward(c, http.MethodPost, agentName, "/api/chat", injectUserID(c.Body(), userID))
}

func (p *ProxyService) ChatStream(c fiber.Ctx, agentName string, userID int) error {
	return p.forward(c, http.MethodPost, agentName, "/api/chat/stream", injectUserID(c.Body(), userID))
}

// ChatStop cancels the run a client started with the same request id.
func (p *ProxyService) ChatStop(c fiber.Ctx, agentName string, userID int) error {
	return p.forward(c, http.MethodPost, agentName, "/api/chat/stop", injectUserID(c.Body(), userID))
}

// ChatRuns lists the runs a reconnecting client can attach to.
func (p *ProxyService) ChatRuns(c fiber.Ctx, agentName string) error {
	return p.forward(c, http.MethodGet, agentName, "/api/chat/runs"+queryString(c), nil)
}

// ChatRunStream relays the events of one run, replaying what the client missed.
func (p *ProxyService) ChatRunStream(c fiber.Ctx, agentName, requestID string) error {
	path := "/api/chat/run/" + url.PathEscape(requestID) + "/stream" + queryString(c)
	return p.forward(c, http.MethodGet, agentName, path, nil)
}

// Events relays the agent's conversation SSE stream: a client subscribes once per
// agent and re-reads a transcript when a turn lands.
func (p *ProxyService) Events(c fiber.Ctx, agentName string) error {
	return p.forward(c, http.MethodGet, agentName, "/api/events"+queryString(c), nil)
}

// EventsAll multiplexes /api/events from all requested (or currently running)
// agents into a single SSE stream so the browser only consumes one HTTP/1.1
// connection regardless of how many agents are running.
func (p *ProxyService) EventsAll(c fiber.Ctx, agentNames []string) error {
	names := make([]string, 0, len(agentNames))
	seen := make(map[string]struct{}, len(agentNames))
	for _, raw := range agentNames {
		name := strings.Clone(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		listed, err := p.agents.List()
		if err == nil {
			for _, info := range listed {
				if info.Running {
					names = append(names, info.Name)
				}
			}
		}
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		// Flush headers immediately so the browser never waits for the first keepalive.
		if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		out := make(chan []byte, 64)
		for _, name := range names {
			go p.relayAgentEvents(ctx, name, out)
		}
		if p.groups != nil {
			groupCh, unsubGroups := p.groups.SubscribeGlobal()
			defer unsubGroups()
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case ev, ok := <-groupCh:
						if !ok {
							return
						}
						encoded, err := json.Marshal(ev.Data)
						if err != nil {
							continue
						}
						block := []byte(fmt.Sprintf("event: group.updated\ndata: %s\n\n", encoded))
						select {
						case out <- block:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
		}

		keepAlive := time.NewTicker(20 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case block := <-out:
				if _, err := w.Write(block); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case <-keepAlive.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
}

// relayAgentEvents tails one agent's /api/events SSE stream and forwards
// non-comment SSE event blocks into out until ctx is cancelled.
func (p *ProxyService) relayAgentEvents(ctx context.Context, agentName string, out chan<- []byte) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		baseURL, _, err := p.agents.BaseURL(agentName)
		if err == nil {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/events", nil)
			if reqErr == nil {
				req.Header.Set("Accept", "text/event-stream")
				resp, doErr := p.client.Do(req)
				if doErr == nil {
					if resp.StatusCode == http.StatusOK {
						backoff = time.Second
						p.pipeSSEBlocks(ctx, resp.Body, out)
					}
					_ = resp.Body.Close()
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff *= 2
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
		}
	}
}

func (p *ProxyService) pipeSSEBlocks(ctx context.Context, body io.Reader, out chan<- []byte) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 16*1024), 256*1024)

	var block bytes.Buffer
	hasData := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if hasData {
				block.WriteByte('\n')
				chunk := append([]byte(nil), block.Bytes()...)
				select {
				case out <- chunk:
				case <-ctx.Done():
					return
				}
			}
			block.Reset()
			hasData = false
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			hasData = true
		}
		block.WriteString(line)
		block.WriteByte('\n')
	}
}

func queryString(c fiber.Ctx) string {
	values := c.Queries()
	if len(values) == 0 {
		return ""
	}
	params := url.Values{}
	for key, value := range values {
		params.Set(key, value)
	}
	return "?" + params.Encode()
}

// ApprovalDecision forwards a tool approval decision to the agent that owns the
// pending run.
func (p *ProxyService) ApprovalDecision(c fiber.Ctx, agentName, approvalID string, userID int) error {
	baseURL, _, err := p.agents.BaseURL(agentName)
	if err != nil {
		return err
	}

	target := baseURL + "/api/approvals/" + url.PathEscape(approvalID)
	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, target, bytes.NewReader(c.Body()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(approvals.UserIDHeader, strconv.Itoa(userID))

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}
	defer resp.Body.Close()

	c.Status(resp.StatusCode)
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Set("Content-Type", contentType)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return c.Send(body)
}

func (p *ProxyService) forward(c fiber.Ctx, method, agentName, path string, body []byte) error {
	baseURL, _, err := p.agents.BaseURL(agentName)
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(c.Context(), method, baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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

	// Streams (chat and run attach) must stay open while the body is relayed;
	// buffering them would empty the response.
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
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
