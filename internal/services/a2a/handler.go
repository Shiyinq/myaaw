package a2a

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Card serves the public Agent Card.
func (h *Handler) Card(c fiber.Ctx) error {
	return c.JSON(h.service.Card())
}

// RPC handles JSON-RPC 2.0 task methods.
func (h *Handler) RPC(c fiber.Ctx) error {
	caller, ok := h.authenticate(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req RPCRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.JSON(errorResponse(nil, CodeParseError, "parse error"))
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		return c.JSON(errorResponse(req.ID, CodeInvalidRequest, "invalid JSON-RPC request"))
	}

	method := normalizeMethod(req.Method)
	if !caller.Can(scopeForMethod(method)) {
		return c.Status(fiber.StatusForbidden).JSON(errorResponse(req.ID, CodeForbidden, "insufficient scope"))
	}

	switch method {
	case "tasks/sendSubscribe":
		var params SendTaskParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid tasks/sendSubscribe params"))
		}
		if !hasContent(params.Message) {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "message must contain text, data, or a file part"))
		}
		if params.Async {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "async is not supported with tasks/sendSubscribe"))
		}
		params.Caller = caller.Name

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.SendStreamWriter(func(w *bufio.Writer) {
			emit := func(event StreamEvent) {
				data, err := json.Marshal(event)
				if err != nil {
					return
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, string(data))
				w.Flush()
			}
			task, err := h.service.SendTaskStream(c.Context(), params, emit)
			if err != nil {
				failure := Message{Role: "agent", Parts: []Part{{Kind: "text", Text: err.Error()}}}
				emit(StreamEvent{
					Kind:   "status",
					TaskID: task.ID,
					Status: &TaskStatus{
						State:     StateFailed,
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						Message:   &failure,
					},
					Final: true,
				})
			}
		})
		return nil

	case "tasks/send":
		var params SendTaskParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid tasks/send params"))
		}
		params.Caller = caller.Name
		task, err := h.service.SendTask(c.Context(), params)
		if err != nil {
			code := CodeInternal
			switch {
			case errors.Is(err, ErrTaskExists):
				code = CodeTaskExists
			case isInvalidParamsError(err):
				code = CodeInvalidParams
			}
			return c.JSON(errorResponse(req.ID, code, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, task.ForResponse()))

	case "tasks/get":
		var params TaskQueryParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid tasks/get params"))
		}
		task, err := h.service.GetTask(c.Context(), params.ID)
		if err != nil {
			code := CodeInternal
			if errors.Is(err, ErrTaskNotFound) {
				code = CodeTaskNotFound
			}
			return c.JSON(errorResponse(req.ID, code, err.Error()))
		}
		if !h.service.OwnsTask(task, caller.Name) {
			return c.JSON(errorResponse(req.ID, CodeTaskNotFound, "task not found"))
		}
		return c.JSON(resultResponse(req.ID, task.ForResponse()))

	case "tasks/cancel":
		var params TaskQueryParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid tasks/cancel params"))
		}
		task, err := h.service.GetTask(c.Context(), params.ID)
		if err != nil {
			code := CodeInternal
			if errors.Is(err, ErrTaskNotFound) {
				code = CodeTaskNotFound
			}
			return c.JSON(errorResponse(req.ID, code, err.Error()))
		}
		if !h.service.OwnsTask(task, caller.Name) {
			return c.JSON(errorResponse(req.ID, CodeTaskNotFound, "task not found"))
		}
		task, err = h.service.CancelTask(c.Context(), params.ID)
		if err != nil {
			code := CodeInternal
			if errors.Is(err, ErrTaskNotFound) {
				code = CodeTaskNotFound
			}
			return c.JSON(errorResponse(req.ID, code, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, task.ForResponse()))

	case "tasks/resubscribe":
		var params TaskQueryParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid tasks/resubscribe params"))
		}
		task, ok := h.ownedTask(c, req.ID, params.ID, caller)
		if !ok {
			return nil
		}
		snapshot, events, active, exists := h.service.subscribeStream(params.ID)

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.SendStreamWriter(func(w *bufio.Writer) {
			emit := func(event StreamEvent) {
				data, err := json.Marshal(event)
				if err != nil {
					return
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, string(data))
				w.Flush()
			}
			if active {
				defer h.service.unsubscribeStream(params.ID, events)
			}

			for _, event := range snapshot {
				emit(event)
			}
			if !active {
				if !exists {
					status := task.Status
					emit(StreamEvent{Kind: "status", TaskID: task.ID, Status: &status, Final: true})
				}
				return
			}
			for event := range events {
				emit(event)
			}
		})
		return nil

	case "tasks/pushNotificationConfig/set":
		var params SetPushConfigParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid pushNotificationConfig/set params"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.ID, caller); !ok {
			return nil
		}
		pushURL, err := validatePushURL(params.PushNotificationConfig.URL)
		if err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, err.Error()))
		}
		config := params.PushNotificationConfig
		config.URL = pushURL
		if config.ID == "" {
			config.ID = uuid.NewString()
		}
		saved, err := h.service.store.SetPushConfig(c.Context(), params.ID, config)
		if err != nil {
			return c.JSON(errorResponse(req.ID, CodeInternal, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, saved))

	case "tasks/pushNotificationConfig/list":
		var params TaskQueryParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid pushNotificationConfig/list params"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.ID, caller); !ok {
			return nil
		}
		configs, err := h.service.store.ListPushConfigs(c.Context(), params.ID)
		if err != nil {
			return c.JSON(errorResponse(req.ID, CodeInternal, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, configs))

	case "tasks/pushNotificationConfig/get":
		var params PushConfigIDParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid pushNotificationConfig/get params"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.ID, caller); !ok {
			return nil
		}
		if params.PushNotificationConfigID == "" {
			configs, err := h.service.store.ListPushConfigs(c.Context(), params.ID)
			if err != nil {
				return c.JSON(errorResponse(req.ID, CodeInternal, err.Error()))
			}
			if len(configs) == 0 {
				return c.JSON(errorResponse(req.ID, CodeTaskNotFound, "push notification config not found"))
			}
			return c.JSON(resultResponse(req.ID, configs[0]))
		}
		config, err := h.service.store.GetPushConfig(c.Context(), params.ID, params.PushNotificationConfigID)
		if err != nil {
			code := CodeInternal
			if errors.Is(err, ErrPushConfigNotFound) {
				code = CodeTaskNotFound
			}
			return c.JSON(errorResponse(req.ID, code, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, config))

	case "tasks/pushNotificationConfig/delete":
		var params PushConfigIDParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "invalid pushNotificationConfig/delete params"))
		}
		if params.PushNotificationConfigID == "" {
			return c.JSON(errorResponse(req.ID, CodeInvalidParams, "pushNotificationConfigId is required"))
		}
		if _, ok := h.ownedTask(c, req.ID, params.ID, caller); !ok {
			return nil
		}
		if err := h.service.store.DeletePushConfig(c.Context(), params.ID, params.PushNotificationConfigID); err != nil {
			code := CodeInternal
			if errors.Is(err, ErrPushConfigNotFound) {
				code = CodeTaskNotFound
			}
			return c.JSON(errorResponse(req.ID, code, err.Error()))
		}
		return c.JSON(resultResponse(req.ID, map[string]bool{"deleted": true}))

	default:
		return c.JSON(errorResponse(req.ID, CodeMethodNotFound, "method not found"))
	}
}

// ownedTask writes a JSON-RPC error and returns false when the task is missing
// or belongs to another caller.
func (h *Handler) ownedTask(c fiber.Ctx, reqID json.RawMessage, id string, caller Caller) (Task, bool) {
	task, err := h.service.GetTask(c.Context(), id)
	if err != nil {
		code := CodeInternal
		if errors.Is(err, ErrTaskNotFound) {
			code = CodeTaskNotFound
		}
		_ = c.JSON(errorResponse(reqID, code, err.Error()))
		return Task{}, false
	}
	if !h.service.OwnsTask(task, caller.Name) {
		_ = c.JSON(errorResponse(reqID, CodeTaskNotFound, "task not found"))
		return Task{}, false
	}
	return task, true
}

func (h *Handler) authenticate(c fiber.Ctx) (Caller, bool) {
	caller, ok := h.service.Authenticate(bearerToken(c))
	if !ok {
		return Caller{}, false
	}

	signature := strings.TrimSpace(c.Get(SignatureHeader))
	if signature != "" {
		name, err := h.service.VerifySignature(signature, c.Body())
		if err != nil {
			log.Printf("a2a: signature rejected: %v", err)
			return Caller{}, false
		}
		if name != "" {
			caller.Name = name
		}
	} else if h.service.requireSignature {
		return Caller{}, false
	}
	return caller, true
}

func bearerToken(c fiber.Ctx) string {
	header := strings.TrimSpace(c.Get("Authorization"))
	if len(header) > 7 && strings.EqualFold(header[:7], "Bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return header
}

// normalizeMethod accepts both the original Myaaw names and the newer A2A
// method names.
func normalizeMethod(method string) string {
	switch strings.TrimSpace(method) {
	case "message/send":
		return "tasks/send"
	case "message/stream":
		return "tasks/sendSubscribe"
	default:
		return strings.TrimSpace(method)
	}
}

func scopeForMethod(method string) string {
	switch method {
	case "tasks/get", "tasks/resubscribe", "tasks/pushNotificationConfig/get", "tasks/pushNotificationConfig/list":
		return ScopeRead
	default:
		return ScopeDelegate
	}
}

func resultResponse(id json.RawMessage, result any) RPCResponse {
	return RPCResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) RPCResponse {
	return RPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}

// isInvalidParamsError reports whether a task error was caused by the caller's
// input rather than by the agent or storage.
func isInvalidParamsError(err error) bool {
	msg := err.Error()
	for _, marker := range []string{
		"message must contain",
		"too many file parts",
		"file URI parts",
		"file part is missing bytes",
		"invalid base64 file bytes",
		"is too large",
		"pushNotificationUrl must",
		"task id may only",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
