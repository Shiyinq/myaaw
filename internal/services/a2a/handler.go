package a2a

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Card serves the public Agent Card with validation caching per spec §3.1.
func (h *Handler) Card(c fiber.Ctx) error {
	card, err := json.Marshal(h.service.Card())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "cannot marshal agent card"})
	}
	sum := sha256.Sum256(card)
	etag := fmt.Sprintf(`"%x"`, sum[:16])

	c.Set("Content-Type", "application/json")
	c.Set("Cache-Control", "no-cache")
	c.Set("ETag", etag)
	if match := c.Get("If-None-Match"); match != "" && etagMatches(match, etag) {
		c.Status(fiber.StatusNotModified)
		return nil
	}
	return c.Send(card)
}

// etagMatches reports whether an If-None-Match header value covers the etag.
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || candidate == "W/"+etag {
			return true
		}
	}
	return false
}

// RPC handles JSON-RPC 2.0 method calls.
func (h *Handler) RPC(c fiber.Ctx) error {
	if err := checkVersion(c); err != nil {
		return c.JSON(versionError(nil, err))
	}

	caller, ok := h.authenticate(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if !h.service.AllowCaller(caller.Name) {
		c.Set("Retry-After", "1")
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "rate limit exceeded"})
	}

	var req RPCRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.JSON(errorResponse(nil, CodeParseError, "parse error"))
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		return c.JSON(errorResponse(req.ID, CodeInvalidRequest, "invalid JSON-RPC request"))
	}
	log.Printf("a2a: audit caller=%q method=%q", caller.Name, req.Method)

	if !caller.Can(scopeForMethod(req.Method)) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "insufficient scope"})
	}

	switch req.Method {
	case "SendMessage":
		var params SendMessageRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid SendMessage params"))
		}
		params.Caller = caller.Name
		task, err := h.service.SendMessage(c.Context(), params)
		if err != nil {
			return c.JSON(sendError(req.ID, err))
		}
		task = trimHistory(task, historyLength(params.Configuration))
		return c.JSON(resultResponse(req.ID, SendMessageResponse{Task: &task}))

	case "SendStreamingMessage":
		var params SendMessageRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid SendStreamingMessage params"))
		}
		params.Caller = caller.Name

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.SendStreamWriter(func(w *bufio.Writer) {
			write := func(payload any) {
				envelope := RPCResponse{JSONRPC: "2.0", ID: req.ID, Result: payload}
				data, err := json.Marshal(envelope)
				if err != nil {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				w.Flush()
			}
			_, err := h.service.SendMessageStream(c.Context(), params, func(response StreamResponse) {
				write(response)
			})
			if err != nil {
				errResponse := sendError(req.ID, err)
				envelope := RPCResponse{JSONRPC: "2.0", ID: req.ID, Error: errResponse.Error}
				if data, marshalErr := json.Marshal(envelope); marshalErr == nil {
					fmt.Fprintf(w, "data: %s\n\n", string(data))
					w.Flush()
				}
			}
		})
		return nil

	case "GetTask":
		var params GetTaskRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid GetTask params"))
		}
		task, ok := h.ownedTask(c, req.ID, params.ID, caller)
		if !ok {
			return nil
		}
		return c.JSON(resultResponse(req.ID, trimHistory(task, params.HistoryLength)))

	case "ListTasks":
		var params ListTasksRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid ListTasks params"))
		}
		response, err := h.service.store.ListTasks(c.Context(), params, caller.Name)
		if err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, err.Error()))
		}
		for i := range response.Tasks {
			response.Tasks[i] = trimHistory(response.Tasks[i], params.HistoryLength)
		}
		return c.JSON(resultResponse(req.ID, response))

	case "CancelTask":
		var params CancelTaskRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid CancelTask params"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.ID, caller); !ok {
			return nil
		}
		task, err := h.service.CancelTask(c.Context(), params.ID)
		if err != nil {
			return c.JSON(sendError(req.ID, err))
		}
		return c.JSON(resultResponse(req.ID, task))

	case "SubscribeToTask":
		var params SubscribeToTaskRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid SubscribeToTask params"))
		}
		task, ok := h.ownedTask(c, req.ID, params.ID, caller)
		if !ok {
			return nil
		}
		if TerminalStates[task.Status.State] {
			return c.JSON(a2aError(req.ID, CodeUnsupportedOperation, "UNSUPPORTED_OPERATION", "task is in a terminal state", nil))
		}
		_, events, active, exists := h.service.subscribeStream(params.ID)

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.SendStreamWriter(func(w *bufio.Writer) {
			write := func(payload any) {
				envelope := RPCResponse{JSONRPC: "2.0", ID: req.ID, Result: payload}
				data, err := json.Marshal(envelope)
				if err != nil {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				w.Flush()
			}
			if active {
				defer h.service.unsubscribeStream(params.ID, events)
			}
			write(StreamResponse{Task: &task})
			if !active {
				if !exists {
					write(StreamResponse{StatusUpdate: &TaskStatusUpdateEvent{
						TaskID:    task.ID,
						ContextID: task.ContextID,
						Status:    task.Status,
					}})
				}
				return
			}
			for response := range events {
				write(response)
			}
		})
		return nil

	case "CreateTaskPushNotificationConfig":
		var params PushNotificationConfig
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid CreateTaskPushNotificationConfig params"))
		}
		if strings.TrimSpace(params.TaskID) == "" {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "taskId is required"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.TaskID, caller); !ok {
			return nil
		}
		pushURL, err := validateWebhookURL(params.URL)
		if err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, err.Error()))
		}
		if strings.TrimSpace(pushURL) == "" {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "url is required"))
		}
		params.URL = pushURL
		if params.ID == "" {
			params.ID = uuid.NewString()
		}
		saved, err := h.service.store.SetPushConfig(c.Context(), params.TaskID, params)
		if err != nil {
			return c.JSON(errorResponse(req.ID, CodeInternal, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, saved))

	case "GetTaskPushNotificationConfig":
		var params GetTaskPushNotificationConfigRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid GetTaskPushNotificationConfig params"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.TaskID, caller); !ok {
			return nil
		}
		config, err := h.service.store.GetPushConfig(c.Context(), params.TaskID, params.ID)
		if err != nil {
			return c.JSON(sendError(req.ID, err))
		}
		return c.JSON(resultResponse(req.ID, config))

	case "ListTaskPushNotificationConfigs":
		var params ListTaskPushNotificationConfigsRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid ListTaskPushNotificationConfigs params"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.TaskID, caller); !ok {
			return nil
		}
		configs, err := h.service.store.ListPushConfigs(c.Context(), params.TaskID)
		if err != nil {
			return c.JSON(errorResponse(req.ID, CodeInternal, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, ListTaskPushNotificationConfigsResponse{Configs: configs}))

	case "DeleteTaskPushNotificationConfig":
		var params DeleteTaskPushNotificationConfigRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid DeleteTaskPushNotificationConfig params"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.TaskID, caller); !ok {
			return nil
		}
		// Deletion is idempotent per spec: a missing config is not an error.
		_ = h.service.store.DeletePushConfig(c.Context(), params.TaskID, params.ID)
		return c.JSON(resultResponse(req.ID, struct{}{}))

	case "GetExtendedAgentCard":
		if !ExtendedCardSupported() {
			return c.JSON(a2aError(req.ID, CodeUnsupportedOperation, "UNSUPPORTED_OPERATION", "extended agent card is not supported", nil))
		}
		card, err := h.service.ExtendedCard()
		if err != nil {
			if errors.Is(err, ErrExtendedCardNotConfigured) {
				return c.JSON(a2aError(req.ID, CodeExtendedAgentCardNotConfigured, "EXTENDED_AGENT_CARD_NOT_CONFIGURED", err.Error(), nil))
			}
			return c.JSON(errorResponse(req.ID, CodeInternal, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, json.RawMessage(card)))

	default:
		return c.JSON(errorResponse(req.ID, CodeMethodNotFound, "method not found"))
	}
}

func (h *Handler) authenticate(c fiber.Ctx) (Caller, bool) {
	return h.service.Authenticate(bearerToken(c))
}

func bearerToken(c fiber.Ctx) string {
	header := strings.TrimSpace(c.Get("Authorization"))
	if len(header) > 7 && strings.EqualFold(header[:7], "Bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return header
}

// checkVersion validates the A2A-Version service parameter.
func checkVersion(c fiber.Ctx) error {
	version := strings.TrimSpace(c.Get("A2A-Version"))
	if version == "" {
		version = strings.TrimSpace(c.Query("A2A-Version"))
	}
	if version == "" {
		version = "0.3"
	}
	if version != SupportedProtocolVersion {
		return fmt.Errorf("protocol version %s is not supported (supported: %s)", version, SupportedProtocolVersion)
	}
	return nil
}

func versionError(id json.RawMessage, err error) RPCResponse {
	return RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    CodeVersionNotSupported,
			Message: err.Error(),
			Data:    []any{errorInfo("VERSION_NOT_SUPPORTED", nil)},
		},
	}
}

func scopeForMethod(method string) string {
	switch method {
	case "GetTask", "ListTasks", "GetTaskPushNotificationConfig", "ListTaskPushNotificationConfigs", "GetExtendedAgentCard":
		return ScopeRead
	default:
		return ScopeDelegate
	}
}

// ownedTask writes a JSON-RPC error and returns false when the task is missing
// or belongs to another caller.
func (h *Handler) ownedTask(c fiber.Ctx, reqID json.RawMessage, id string, caller Caller) (Task, bool) {
	task, err := h.service.GetTask(c.Context(), id)
	if err != nil {
		_ = c.JSON(sendError(reqID, err))
		return Task{}, false
	}
	if !h.service.OwnsTask(task, caller.Name) {
		_ = c.JSON(a2aError(reqID, CodeTaskNotFound, "TASK_NOT_FOUND", "task not found", map[string]string{"taskId": id}))
		return Task{}, false
	}
	return task, true
}

func historyLength(config *SendMessageConfiguration) *int {
	if config == nil {
		return nil
	}
	return config.HistoryLength
}

// trimHistory applies historyLength semantics: nil keeps everything, 0 omits
// the history, and n > 0 keeps the n most recent messages.
func trimHistory(task Task, length *int) Task {
	if length == nil {
		return task
	}
	if *length <= 0 {
		task.Messages = nil
		return task
	}
	if len(task.Messages) > *length {
		task.Messages = task.Messages[len(task.Messages)-*length:]
	}
	return task
}

func resultResponse(id json.RawMessage, result any) RPCResponse {
	return RPCResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) RPCResponse {
	return RPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}

// a2aError builds an A2A-specific error with an ErrorInfo detail object.
func a2aError(id json.RawMessage, code int, reason, message string, metadata map[string]string) RPCResponse {
	return RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    []any{errorInfo(reason, metadata)},
		},
	}
}

// sendError maps service errors to spec error responses.
func sendError(id json.RawMessage, err error) RPCResponse {
	switch {
	case errors.Is(err, ErrTaskNotFound):
		return a2aError(id, CodeTaskNotFound, "TASK_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, ErrTaskNotCancelable):
		return a2aError(id, CodeTaskNotCancelable, "TASK_NOT_CANCELABLE", err.Error(), nil)
	case errors.Is(err, ErrTaskNotResumable):
		return a2aError(id, CodeUnsupportedOperation, "UNSUPPORTED_OPERATION", err.Error(), nil)
	case errors.Is(err, ErrContextMismatch):
		return errorResponse(id, CodeInvalidParams, err.Error())
	case errors.Is(err, ErrPushConfigNotFound):
		return a2aError(id, CodeTaskNotFound, "TASK_NOT_FOUND", err.Error(), nil)
	case isInvalidParamsError(err):
		return errorResponse(id, CodeInvalidParams, err.Error())
	default:
		return errorResponse(id, CodeInternal, err.Error())
	}
}

// isInvalidParamsError reports whether a task error was caused by the caller's
// input rather than by the agent or storage.
func isInvalidParamsError(err error) bool {
	msg := err.Error()
	for _, marker := range []string{
		"message must contain",
		"too many file parts",
		"is too large",
		"task id may only",
		"part ",
		"url must be an absolute",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
