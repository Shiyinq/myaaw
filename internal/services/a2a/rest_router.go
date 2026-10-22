package a2a

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// restStatus is the google.rpc.Status JSON shape used by the HTTP+JSON
// binding's error responses (spec §11.6).
type restStatus struct {
	Code    int         `json:"code"`
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Details []ErrorInfo `json:"details,omitempty"`
}

// mountRest mounts the HTTP+JSON (REST) binding endpoints on the same router,
// reusing the shared service and handler.
func mountRest(router fiber.Router, h *Handler) {
	router.Post(`/message\:send`, h.restSendMessage)
	router.Post(`/message\:stream`, h.restSendStreamingMessage)
	router.Get("/tasks", h.restListTasks)
	router.Get("/tasks/:id", h.restGetTask)
	router.Post(`/tasks/:id\:cancel`, h.restCancelTask)
	router.Post(`/tasks/:id\:subscribe`, h.restSubscribeToTask)
	router.Post("/tasks/:id/pushNotificationConfigs", h.restCreatePushConfig)
	router.Get("/tasks/:id/pushNotificationConfigs", h.restListPushConfigs)
	router.Get("/tasks/:id/pushNotificationConfigs/:configId", h.restGetPushConfig)
	router.Delete("/tasks/:id/pushNotificationConfigs/:configId", h.restDeletePushConfig)
	router.Get("/extendedAgentCard", h.restExtendedAgentCard)
}

// restAuth performs version and bearer authentication, writing a REST error
// response when they fail.
func (h *Handler) restAuth(c fiber.Ctx, scope string) (Caller, bool) {
	if err := checkVersion(c); err != nil {
		writeRestStatus(c, fiber.StatusBadRequest, "FAILED_PRECONDITION", "VERSION_NOT_SUPPORTED", err.Error(), nil)
		return Caller{}, false
	}
	caller, ok := h.authenticate(c)
	if !ok {
		writeRestStatus(c, fiber.StatusUnauthorized, "UNAUTHENTICATED", "", "unauthorized", nil)
		return Caller{}, false
	}
	if !h.service.AllowCaller(caller.Name) {
		c.Set("Retry-After", "1")
		writeRestStatus(c, fiber.StatusTooManyRequests, "RESOURCE_EXHAUSTED", "", "rate limit exceeded", nil)
		return Caller{}, false
	}
	if !caller.Can(scope) {
		writeRestStatus(c, fiber.StatusForbidden, "PERMISSION_DENIED", "", "insufficient scope", nil)
		return Caller{}, false
	}
	return caller, true
}

// restOwnedTask resolves a task by path id and enforces caller ownership,
// writing a REST error when it fails.
func (h *Handler) restOwnedTask(c fiber.Ctx, id string, caller Caller) (Task, bool) {
	task, err := h.service.GetTask(c.Context(), id)
	if err != nil {
		writeRestServiceError(c, err)
		return Task{}, false
	}
	if !h.service.OwnsTask(task, caller.Name) {
		writeRestStatus(c, fiber.StatusNotFound, "NOT_FOUND", "TASK_NOT_FOUND", "task not found", map[string]string{"taskId": id})
		return Task{}, false
	}
	return task, true
}

// restSendMessage implements POST /message:send.
func (h *Handler) restSendMessage(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeDelegate)
	if !ok {
		return nil
	}

	var params SendMessageRequest
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		writeRestStatus(c, fiber.StatusBadRequest, "INVALID_ARGUMENT", "", "invalid SendMessage request body", nil)
		return nil
	}
	params.Caller = caller.Name
	task, err := h.service.SendMessage(c.Context(), params)
	if err != nil {
		writeRestServiceError(c, err)
		return nil
	}
	task = trimHistory(task, historyLength(params.Configuration))
	return writeRestJSON(c, fiber.StatusOK, SendMessageResponse{Task: &task})
}

// restSendStreamingMessage implements POST /message:stream.
func (h *Handler) restSendStreamingMessage(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeDelegate)
	if !ok {
		return nil
	}

	var params SendMessageRequest
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		writeRestStatus(c, fiber.StatusBadRequest, "INVALID_ARGUMENT", "", "invalid SendMessage request body", nil)
		return nil
	}
	params.Caller = caller.Name

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	emitted := false
	c.SendStreamWriter(func(w *bufio.Writer) {
		write := func(response StreamResponse) {
			data, err := json.Marshal(response)
			if err != nil {
				return
			}
			emitted = true
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			w.Flush()
		}
		_, err := h.service.SendMessageStream(c.Context(), params, write)
		if err != nil && !emitted {
			writeRestStreamError(w, err)
		}
	})
	return nil
}

// restGetTask implements GET /tasks/{id}.
func (h *Handler) restGetTask(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeRead)
	if !ok {
		return nil
	}

	task, ok := h.restOwnedTask(c, c.Params("id"), caller)
	if !ok {
		return nil
	}
	length := queryHistoryLength(c)
	return writeRestJSON(c, fiber.StatusOK, trimHistory(task, length))
}

// restListTasks implements GET /tasks.
func (h *Handler) restListTasks(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeRead)
	if !ok {
		return nil
	}

	params, err := listTasksRequestFromQuery(c)
	if err != nil {
		writeRestStatus(c, fiber.StatusBadRequest, "INVALID_ARGUMENT", "", err.Error(), nil)
		return nil
	}
	response, err := h.service.store.ListTasks(c.Context(), params, caller.Name)
	if err != nil {
		writeRestStatus(c, fiber.StatusBadRequest, "INVALID_ARGUMENT", "", err.Error(), nil)
		return nil
	}
	for i := range response.Tasks {
		response.Tasks[i] = trimHistory(response.Tasks[i], params.HistoryLength)
	}
	return writeRestJSON(c, fiber.StatusOK, response)
}

// restCancelTask implements POST /tasks/{id}:cancel.
func (h *Handler) restCancelTask(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeDelegate)
	if !ok {
		return nil
	}

	id := c.Params("id")
	if _, ok := h.restOwnedTask(c, id, caller); !ok {
		return nil
	}
	task, err := h.service.CancelTask(c.Context(), id)
	if err != nil {
		writeRestServiceError(c, err)
		return nil
	}
	return writeRestJSON(c, fiber.StatusOK, task)
}

// restSubscribeToTask implements POST /tasks/{id}:subscribe.
func (h *Handler) restSubscribeToTask(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeDelegate)
	if !ok {
		return nil
	}

	id := c.Params("id")
	task, ok := h.restOwnedTask(c, id, caller)
	if !ok {
		return nil
	}
	if TerminalStates[task.Status.State] {
		writeRestStatus(c, fiber.StatusBadRequest, "FAILED_PRECONDITION", "UNSUPPORTED_OPERATION", "task is in a terminal state", nil)
		return nil
	}
	_, events, active, exists := h.service.subscribeStream(id)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.SendStreamWriter(func(w *bufio.Writer) {
		write := func(response StreamResponse) {
			data, err := json.Marshal(response)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			w.Flush()
		}
		if active {
			defer h.service.unsubscribeStream(id, events)
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
}

// restCreatePushConfig implements POST /tasks/{id}/pushNotificationConfigs.
func (h *Handler) restCreatePushConfig(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeDelegate)
	if !ok {
		return nil
	}

	taskID := c.Params("id")
	if _, ok := h.restOwnedTask(c, taskID, caller); !ok {
		return nil
	}

	var params PushNotificationConfig
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		writeRestStatus(c, fiber.StatusBadRequest, "INVALID_ARGUMENT", "", "invalid PushNotificationConfig body", nil)
		return nil
	}
	pushURL, err := validateWebhookURL(params.URL)
	if err != nil {
		writeRestStatus(c, fiber.StatusBadRequest, "INVALID_ARGUMENT", "", err.Error(), nil)
		return nil
	}
	if strings.TrimSpace(pushURL) == "" {
		writeRestStatus(c, fiber.StatusBadRequest, "INVALID_ARGUMENT", "", "url is required", nil)
		return nil
	}
	params.URL = pushURL
	params.TaskID = taskID
	if params.ID == "" {
		params.ID = uuid.NewString()
	}
	saved, err := h.service.store.SetPushConfig(c.Context(), taskID, params)
	if err != nil {
		writeRestStatus(c, fiber.StatusInternalServerError, "INTERNAL", "", err.Error(), nil)
		return nil
	}
	return writeRestJSON(c, fiber.StatusOK, saved)
}

// restGetPushConfig implements GET /tasks/{id}/pushNotificationConfigs/{configId}.
func (h *Handler) restGetPushConfig(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeRead)
	if !ok {
		return nil
	}

	taskID := c.Params("id")
	if _, ok := h.restOwnedTask(c, taskID, caller); !ok {
		return nil
	}
	config, err := h.service.store.GetPushConfig(c.Context(), taskID, c.Params("configId"))
	if err != nil {
		writeRestServiceError(c, err)
		return nil
	}
	return writeRestJSON(c, fiber.StatusOK, config)
}

// restListPushConfigs implements GET /tasks/{id}/pushNotificationConfigs.
func (h *Handler) restListPushConfigs(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeRead)
	if !ok {
		return nil
	}

	taskID := c.Params("id")
	if _, ok := h.restOwnedTask(c, taskID, caller); !ok {
		return nil
	}
	configs, err := h.service.store.ListPushConfigs(c.Context(), taskID)
	if err != nil {
		writeRestStatus(c, fiber.StatusInternalServerError, "INTERNAL", "", err.Error(), nil)
		return nil
	}
	return writeRestJSON(c, fiber.StatusOK, ListTaskPushNotificationConfigsResponse{Configs: configs})
}

// restDeletePushConfig implements DELETE /tasks/{id}/pushNotificationConfigs/{configId}.
func (h *Handler) restDeletePushConfig(c fiber.Ctx) error {
	caller, ok := h.restAuth(c, ScopeDelegate)
	if !ok {
		return nil
	}

	taskID := c.Params("id")
	if _, ok := h.restOwnedTask(c, taskID, caller); !ok {
		return nil
	}
	_ = h.service.store.DeletePushConfig(c.Context(), taskID, c.Params("configId"))
	return writeRestJSON(c, fiber.StatusOK, struct{}{})
}

// restExtendedAgentCard implements GET /extendedAgentCard.
func (h *Handler) restExtendedAgentCard(c fiber.Ctx) error {
	if _, ok := h.restAuth(c, ScopeRead); !ok {
		return nil
	}
	if !ExtendedCardSupported() {
		writeRestStatus(c, fiber.StatusBadRequest, "FAILED_PRECONDITION", "UNSUPPORTED_OPERATION", "extended agent card is not supported", nil)
		return nil
	}
	card, err := h.service.ExtendedCard()
	if err != nil {
		writeRestServiceError(c, err)
		return nil
	}
	c.Set("Content-Type", "application/a2a+json")
	return c.Send(card)
}

// writeRestJSON writes a successful REST response with the a2a+json type.
func writeRestJSON(c fiber.Ctx, status int, payload any) error {
	c.Set("Content-Type", "application/a2a+json")
	return c.Status(status).JSON(payload)
}

// writeRestStatus writes a google.rpc.Status error response.
func writeRestStatus(c fiber.Ctx, httpCode int, grpcStatus, reason, message string, metadata map[string]string) error {
	body := struct {
		Error restStatus `json:"error"`
	}{
		Error: restStatus{Code: httpCode, Status: grpcStatus, Message: message},
	}
	if reason != "" {
		body.Error.Details = []ErrorInfo{errorInfo(reason, metadata)}
	}
	c.Set("Content-Type", "application/a2a+json")
	return c.Status(httpCode).JSON(body)
}

// writeRestServiceError maps a service/store error to a REST error response.
func writeRestServiceError(c fiber.Ctx, err error) error {
	httpCode, grpcStatus, reason := restErrorMapping(err)
	return writeRestStatus(c, httpCode, grpcStatus, reason, err.Error(), nil)
}

// restErrorMapping maps A2A errors to (HTTP status, gRPC status, reason).
func restErrorMapping(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrTaskNotFound), errors.Is(err, ErrPushConfigNotFound):
		return fiber.StatusNotFound, "NOT_FOUND", "TASK_NOT_FOUND"
	case errors.Is(err, ErrTaskNotCancelable):
		return fiber.StatusBadRequest, "FAILED_PRECONDITION", "TASK_NOT_CANCELABLE"
	case errors.Is(err, ErrTaskNotResumable):
		return fiber.StatusBadRequest, "FAILED_PRECONDITION", "UNSUPPORTED_OPERATION"
	case errors.Is(err, ErrExtendedCardNotConfigured):
		return fiber.StatusBadRequest, "FAILED_PRECONDITION", "EXTENDED_AGENT_CARD_NOT_CONFIGURED"
	case errors.Is(err, ErrContextMismatch):
		return fiber.StatusBadRequest, "INVALID_ARGUMENT", ""
	case isInvalidParamsError(err):
		return fiber.StatusBadRequest, "INVALID_ARGUMENT", ""
	default:
		return fiber.StatusInternalServerError, "INTERNAL", ""
	}
}

// writeRestStreamError writes a google.rpc.Status as a terminal SSE data line.
func writeRestStreamError(w *bufio.Writer, err error) {
	httpCode, grpcStatus, reason := restErrorMapping(err)
	body := struct {
		Error restStatus `json:"error"`
	}{
		Error: restStatus{Code: httpCode, Status: grpcStatus, Message: err.Error()},
	}
	if reason != "" {
		body.Error.Details = []ErrorInfo{errorInfo(reason, nil)}
	}
	if data, marshalErr := json.Marshal(body); marshalErr == nil {
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		w.Flush()
	}
}

func queryHistoryLength(c fiber.Ctx) *int {
	raw := strings.TrimSpace(c.Query("historyLength"))
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		value = 0
	}
	return &value
}

// listTasksRequestFromQuery builds a ListTasksRequest from the GET /tasks
// query parameters (camelCase, spec §11.5).
func listTasksRequestFromQuery(c fiber.Ctx) (ListTasksRequest, error) {
	var req ListTasksRequest
	req.ContextID = c.Query("contextId")
	req.Status = c.Query("status")
	req.PageToken = c.Query("pageToken")
	req.StatusTimestampAfter = c.Query("statusTimestampAfter")
	req.IncludeArtifacts, _ = strconv.ParseBool(c.Query("includeArtifacts"))

	if raw := strings.TrimSpace(c.Query("pageSize")); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil {
			return req, fmt.Errorf("pageSize must be an integer")
		}
		req.PageSize = &size
	}
	if raw := strings.TrimSpace(c.Query("historyLength")); raw != "" {
		length, err := strconv.Atoi(raw)
		if err != nil || length < 0 {
			return req, fmt.Errorf("historyLength must be a non-negative integer")
		}
		req.HistoryLength = &length
	}
	return req, nil
}
