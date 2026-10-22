// Package a2a implements the server side of the Agent2Agent protocol for one
// Myaaw agent: discovery through an Agent Card and task operations over
// JSON-RPC 2.0, following the A2A specification v1.0.
package a2a

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Task states as stored internally. The wire representation is the A2A
// TASK_STATE_* enum; conversion happens at the marshaling boundary.
const (
	StateUnspecified   = "unspecified"
	StateSubmitted     = "submitted"
	StateWorking       = "working"
	StateInputRequired = "input-required"
	StateAuthRequired  = "auth-required"
	StateCompleted     = "completed"
	StateFailed        = "failed"
	StateCanceled      = "canceled"
	StateRejected      = "rejected"
)

// TerminalStates lists states that end a task.
var TerminalStates = map[string]bool{
	StateCompleted: true,
	StateFailed:    true,
	StateCanceled:  true,
	StateRejected:  true,
}

// InterruptedStates pause a task waiting for the caller.
var InterruptedStates = map[string]bool{
	StateInputRequired: true,
	StateAuthRequired:  true,
}

// WireState converts an internal state to the spec enum value.
func WireState(state string) string {
	switch state {
	case StateSubmitted:
		return "TASK_STATE_SUBMITTED"
	case StateWorking:
		return "TASK_STATE_WORKING"
	case StateInputRequired:
		return "TASK_STATE_INPUT_REQUIRED"
	case StateAuthRequired:
		return "TASK_STATE_AUTH_REQUIRED"
	case StateCompleted:
		return "TASK_STATE_COMPLETED"
	case StateFailed:
		return "TASK_STATE_FAILED"
	case StateCanceled:
		return "TASK_STATE_CANCELED"
	case StateRejected:
		return "TASK_STATE_REJECTED"
	default:
		return "TASK_STATE_UNSPECIFIED"
	}
}

// StateFromWire converts a spec enum value (or an internal lowercase value,
// for data read from older storage) to the internal representation.
func StateFromWire(wire string) string {
	switch strings.TrimSpace(wire) {
	case "TASK_STATE_SUBMITTED":
		return StateSubmitted
	case "TASK_STATE_WORKING":
		return StateWorking
	case "TASK_STATE_INPUT_REQUIRED":
		return StateInputRequired
	case "TASK_STATE_AUTH_REQUIRED":
		return StateAuthRequired
	case "TASK_STATE_COMPLETED":
		return StateCompleted
	case "TASK_STATE_FAILED":
		return StateFailed
	case "TASK_STATE_CANCELED":
		return StateCanceled
	case "TASK_STATE_REJECTED":
		return StateRejected
	default:
		return strings.ToLower(wire)
	}
}

// WireRole converts an internal role to the spec enum value.
func WireRole(role string) string {
	switch strings.ToLower(role) {
	case "user":
		return "ROLE_USER"
	case "agent":
		return "ROLE_AGENT"
	default:
		return "ROLE_UNSPECIFIED"
	}
}

// RoleFromWire converts a spec enum value (or a lowercase legacy value) to
// the internal representation.
func RoleFromWire(role string) string {
	switch strings.TrimSpace(role) {
	case "ROLE_USER":
		return "user"
	case "ROLE_AGENT":
		return "agent"
	default:
		return strings.ToLower(role)
	}
}

// SupportedProtocolVersion is the only A2A protocol version accepted.
const SupportedProtocolVersion = "1.0"

// Scopes control what a caller may do. Delegate implies read.
const (
	ScopeRead     = "read"
	ScopeDelegate = "delegate"
)

// Caller is an authenticated A2A client.
type Caller struct {
	Name   string
	Scopes map[string]bool
}

// Can reports whether the caller holds a scope. Delegate implies read.
func (c Caller) Can(scope string) bool {
	if c.Scopes[scope] {
		return true
	}
	return scope == ScopeRead && c.Scopes[ScopeDelegate]
}

// AgentCard is the public discovery document served at
// /.well-known/agent-card.json per spec §4.4.1. It never contains credentials.
type AgentCard struct {
	Name                 string                    `json:"name"`
	Description          string                    `json:"description"`
	SupportedInterfaces  []AgentInterface          `json:"supportedInterfaces"`
	Provider             *AgentProvider            `json:"provider,omitempty"`
	Version              string                    `json:"version"`
	DocumentationURL     string                    `json:"documentationUrl,omitempty"`
	Capabilities         Capabilities              `json:"capabilities"`
	SecuritySchemes      map[string]SecurityScheme `json:"securitySchemes,omitempty"`
	SecurityRequirements []SecurityRequirement     `json:"securityRequirements,omitempty"`
	DefaultInputModes    []string                  `json:"defaultInputModes"`
	DefaultOutputModes   []string                  `json:"defaultOutputModes"`
	Skills               []AgentSkill              `json:"skills"`
	Signatures           []CardSignature           `json:"signatures,omitempty"`
	IconURL              string                    `json:"iconUrl,omitempty"`
}

// AgentInterface declares one transport endpoint of the agent, spec §4.4.6.
// The first entry of SupportedInterfaces is the preferred one.
type AgentInterface struct {
	URL             string `json:"url"`
	ProtocolBinding string `json:"protocolBinding"`
	Tenant          string `json:"tenant,omitempty"`
	ProtocolVersion string `json:"protocolVersion"`
}

// AgentProvider identifies the organization operating the agent, spec §4.4.2.
type AgentProvider struct {
	URL          string `json:"url"`
	Organization string `json:"organization"`
}

// CardSignature is a compact JWS over the canonical Agent Card document.
type CardSignature struct {
	Protected string `json:"protected"`
	Signature string `json:"signature"`
	Header    any    `json:"header,omitempty"`
}

// Capabilities is the AgentCapabilities object, spec §4.4.3.
type Capabilities struct {
	Streaming         bool             `json:"streaming,omitempty"`
	PushNotifications bool             `json:"pushNotifications,omitempty"`
	Extensions        []AgentExtension `json:"extensions,omitempty"`
	ExtendedAgentCard bool             `json:"extendedAgentCard,omitempty"`
}

// AgentExtension declares a protocol extension, spec §4.4.4.
type AgentExtension struct {
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Params      any    `json:"params,omitempty"`
}

// AgentSkill describes one focused capability of the agent, spec §4.4.5.
type AgentSkill struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Description          string                `json:"description"`
	Tags                 []string              `json:"tags"`
	Examples             []string              `json:"examples,omitempty"`
	InputModes           []string              `json:"inputModes,omitempty"`
	OutputModes          []string              `json:"outputModes,omitempty"`
	SecurityRequirements []SecurityRequirement `json:"securityRequirements,omitempty"`
}

// SecurityScheme is the discriminated union of spec §4.5.1. Exactly one of the
// scheme fields must be set.
type SecurityScheme struct {
	APIKeySecurityScheme        *APIKeySecurityScheme        `json:"apiKeySecurityScheme,omitempty"`
	HTTPAuthSecurityScheme      *HTTPAuthSecurityScheme      `json:"httpAuthSecurityScheme,omitempty"`
	OAuth2SecurityScheme        *OAuth2SecurityScheme        `json:"oauth2SecurityScheme,omitempty"`
	OpenIDConnectSecurityScheme *OpenIDConnectSecurityScheme `json:"openIdConnectSecurityScheme,omitempty"`
	MutualTLSSecurityScheme     *MutualTLSSecurityScheme     `json:"mtlsSecurityScheme,omitempty"`
}

// BearerSecurityScheme builds the security scheme Myaaw uses for A2A tokens.
func BearerSecurityScheme() SecurityScheme {
	return SecurityScheme{HTTPAuthSecurityScheme: &HTTPAuthSecurityScheme{Scheme: "bearer"}}
}

// APIKeySecurityScheme is spec §4.5.2.
type APIKeySecurityScheme struct {
	Description string `json:"description,omitempty"`
	Location    string `json:"location"`
	Name        string `json:"name"`
}

// HTTPAuthSecurityScheme is spec §4.5.3.
type HTTPAuthSecurityScheme struct {
	Description  string `json:"description,omitempty"`
	Scheme       string `json:"scheme"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

// OAuth2SecurityScheme is spec §4.5.4.
type OAuth2SecurityScheme struct {
	Description       string `json:"description,omitempty"`
	Flows             any    `json:"flows"`
	OAuth2MetadataURL string `json:"oauth2MetadataUrl,omitempty"`
}

// OpenIDConnectSecurityScheme is spec §4.5.5.
type OpenIDConnectSecurityScheme struct {
	Description      string `json:"description,omitempty"`
	OpenIDConnectURL string `json:"openIdConnectUrl"`
}

// MutualTLSSecurityScheme is spec §4.5.6.
type MutualTLSSecurityScheme struct {
	Description string `json:"description,omitempty"`
}

// SecurityRequirement binds operations or skills to security schemes with
// required scopes, spec §4.5.
type SecurityRequirement struct {
	Schemes map[string]StringList `json:"schemes"`
}

// StringList is the scopes container of a SecurityRequirement.
type StringList struct {
	List []string `json:"list"`
}

// BearerSecurityRequirement requires the bearer scheme with no extra scopes.
func BearerSecurityRequirement() SecurityRequirement {
	return SecurityRequirement{Schemes: map[string]StringList{"bearer": {}}}
}

// Task is the unit of work exchanged between agents. Messages is the internal
// transcript; it is serialized on the wire as the spec field "history".
type Task struct {
	ID        string     `json:"id"`
	ContextID string     `json:"contextId,omitempty"`
	Status    TaskStatus `json:"status"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	Metadata  any        `json:"metadata,omitempty"`

	// Messages is the internal transcript, exposed on the wire as history.
	Messages []Message `json:"-"`

	// Internal fields, never exposed to callers.
	Attempts int    `json:"-"`
	PushURL  string `json:"-"`
	Caller   string `json:"-"`
}

// MarshalJSON emits the spec Task shape: the transcript becomes "history".
func (t Task) MarshalJSON() ([]byte, error) {
	wire := struct {
		ID        string     `json:"id"`
		ContextID string     `json:"contextId,omitempty"`
		Status    TaskStatus `json:"status"`
		Artifacts []Artifact `json:"artifacts,omitempty"`
		History   []Message  `json:"history,omitempty"`
		Metadata  any        `json:"metadata,omitempty"`
	}{
		ID:        t.ID,
		ContextID: t.ContextID,
		Status:    t.Status,
		Artifacts: t.Artifacts,
		History:   t.Messages,
		Metadata:  t.Metadata,
	}
	return json.Marshal(wire)
}

// UnmarshalJSON reads the spec Task shape; "history" is stored into the
// internal Messages transcript.
func (t *Task) UnmarshalJSON(data []byte) error {
	wire := struct {
		ID        string     `json:"id"`
		ContextID string     `json:"contextId,omitempty"`
		Status    TaskStatus `json:"status"`
		Artifacts []Artifact `json:"artifacts,omitempty"`
		History   []Message  `json:"history,omitempty"`
		Messages  []Message  `json:"messages,omitempty"`
		Metadata  any        `json:"metadata,omitempty"`
	}{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	t.ID = wire.ID
	t.ContextID = wire.ContextID
	t.Status = wire.Status
	t.Artifacts = wire.Artifacts
	t.Metadata = wire.Metadata
	t.Messages = wire.History
	if len(t.Messages) == 0 {
		t.Messages = wire.Messages
	}
	return nil
}

type TaskStatus struct {
	State     string   `json:"state"`
	Timestamp string   `json:"timestamp,omitempty"`
	Message   *Message `json:"message,omitempty"`
}

// MarshalJSON converts the internal state to the TASK_STATE_* enum.
func (s TaskStatus) MarshalJSON() ([]byte, error) {
	wire := struct {
		State     string   `json:"state"`
		Timestamp string   `json:"timestamp,omitempty"`
		Message   *Message `json:"message,omitempty"`
	}{
		State:     WireState(s.State),
		Timestamp: s.Timestamp,
		Message:   s.Message,
	}
	return json.Marshal(wire)
}

func (s *TaskStatus) UnmarshalJSON(data []byte) error {
	wire := struct {
		State     string   `json:"state"`
		Timestamp string   `json:"timestamp,omitempty"`
		Message   *Message `json:"message,omitempty"`
	}{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.State = StateFromWire(wire.State)
	s.Timestamp = wire.Timestamp
	s.Message = wire.Message
	return nil
}

type Message struct {
	MessageID string `json:"messageId,omitempty"`
	ContextID string `json:"contextId,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	Role      string `json:"role"`
	Parts     []Part `json:"parts"`
	Metadata  any    `json:"metadata,omitempty"`
	// Extensions lists extension URIs contributed to this message.
	Extensions []string `json:"extensions,omitempty"`
	// ReferenceTaskIds lists tasks this message references for context.
	ReferenceTaskIds []string `json:"referenceTaskIds,omitempty"`
}

// MarshalJSON converts the internal role to the ROLE_* enum.
func (m Message) MarshalJSON() ([]byte, error) {
	wire := struct {
		MessageID        string   `json:"messageId,omitempty"`
		ContextID        string   `json:"contextId,omitempty"`
		TaskID           string   `json:"taskId,omitempty"`
		Role             string   `json:"role"`
		Parts            []Part   `json:"parts"`
		Metadata         any      `json:"metadata,omitempty"`
		Extensions       []string `json:"extensions,omitempty"`
		ReferenceTaskIds []string `json:"referenceTaskIds,omitempty"`
	}{
		MessageID:        m.MessageID,
		ContextID:        m.ContextID,
		TaskID:           m.TaskID,
		Role:             WireRole(m.Role),
		Parts:            m.Parts,
		Metadata:         m.Metadata,
		Extensions:       m.Extensions,
		ReferenceTaskIds: m.ReferenceTaskIds,
	}
	return json.Marshal(wire)
}

func (m *Message) UnmarshalJSON(data []byte) error {
	wire := struct {
		MessageID        string   `json:"messageId,omitempty"`
		ContextID        string   `json:"contextId,omitempty"`
		TaskID           string   `json:"taskId,omitempty"`
		Role             string   `json:"role"`
		Parts            []Part   `json:"parts"`
		Metadata         any      `json:"metadata,omitempty"`
		Extensions       []string `json:"extensions,omitempty"`
		ReferenceTaskIds []string `json:"referenceTaskIds,omitempty"`
	}{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	m.MessageID = wire.MessageID
	m.ContextID = wire.ContextID
	m.TaskID = wire.TaskID
	m.Role = RoleFromWire(wire.Role)
	m.Parts = wire.Parts
	m.Metadata = wire.Metadata
	m.Extensions = wire.Extensions
	m.ReferenceTaskIds = wire.ReferenceTaskIds
	return nil
}

// Part is one piece of a message or artifact. Exactly one of Text, Raw, URL,
// or Data must be set (spec §4.1.6).
type Part struct {
	Text      string `json:"text,omitempty"`
	Raw       []byte `json:"raw,omitempty"` // base64 in JSON
	URL       string `json:"url,omitempty"`
	Data      any    `json:"data,omitempty"`
	Filename  string `json:"filename,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Metadata  any    `json:"metadata,omitempty"`
}

// HasContent reports whether the part carries one of the four content kinds.
func (p Part) HasContent() bool {
	return strings.TrimSpace(p.Text) != "" || len(p.Raw) > 0 || strings.TrimSpace(p.URL) != "" || p.Data != nil
}

// Count returns how many content kinds are set (must be exactly one).
func (p Part) Count() int {
	count := 0
	for _, set := range []bool{
		strings.TrimSpace(p.Text) != "",
		len(p.Raw) > 0,
		strings.TrimSpace(p.URL) != "",
		p.Data != nil,
	} {
		if set {
			count++
		}
	}
	return count
}

type Artifact struct {
	ArtifactID  string   `json:"artifactId"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Parts       []Part   `json:"parts"`
	Metadata    any      `json:"metadata,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
}

// JSON-RPC 2.0 envelopes.

type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []any  `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// A2A-specific error codes per spec §5.4.
const (
	CodeTaskNotFound                   = -32001
	CodeTaskNotCancelable              = -32002
	CodePushNotificationNotSupported   = -32003
	CodeUnsupportedOperation           = -32004
	CodeContentTypeNotSupported        = -32005
	CodeInvalidAgentResponse           = -32006
	CodeExtendedAgentCardNotConfigured = -32007
	CodeExtensionSupportRequired       = -32008
	CodeVersionNotSupported            = -32009
)

// ErrorInfo is the google.rpc.ErrorInfo well-known type carried in
// JSON-RPC error.data per spec §9.5.
type ErrorInfo struct {
	Type     string            `json:"@type"`
	Reason   string            `json:"reason"`
	Domain   string            `json:"domain"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// errorInfo builds the ErrorInfo detail object for an A2A-specific error.
func errorInfo(reason string, metadata map[string]string) ErrorInfo {
	return ErrorInfo{
		Type:     "type.googleapis.com/google.rpc.ErrorInfo",
		Reason:   reason,
		Domain:   "a2a-protocol.org",
		Metadata: metadata,
	}
}

// SendMessageRequest is the params object of the SendMessage method.
type SendMessageRequest struct {
	Tenant        string                    `json:"tenant,omitempty"`
	Message       Message                   `json:"message"`
	Configuration *SendMessageConfiguration `json:"configuration,omitempty"`
	Metadata      any                       `json:"metadata,omitempty"`

	// Caller is set by the handler after authentication.
	Caller string `json:"-"`
}

// SendMessageConfiguration configures one send operation.
type SendMessageConfiguration struct {
	AcceptedOutputModes []string `json:"acceptedOutputModes,omitempty"`
	// TaskPushNotificationConfig registers a webhook for the task. Its id and
	// taskId must be empty when sent inside SendMessage.
	TaskPushNotificationConfig *PushNotificationConfig `json:"taskPushNotificationConfig,omitempty"`
	// HistoryLength limits messages returned in the response history.
	HistoryLength *int `json:"historyLength,omitempty"`
	// ReturnImmediately returns the task as soon as it is created instead of
	// waiting for a terminal or interrupted state.
	ReturnImmediately bool `json:"returnImmediately,omitempty"`
}

// SendMessageResponse contains exactly one of Task or Message.
type SendMessageResponse struct {
	Task    *Task    `json:"task,omitempty"`
	Message *Message `json:"message,omitempty"`
}

// GetTaskRequest is the params object of the GetTask method.
type GetTaskRequest struct {
	Tenant        string `json:"tenant,omitempty"`
	ID            string `json:"id"`
	HistoryLength *int   `json:"historyLength,omitempty"`
}

// CancelTaskRequest is the params object of the CancelTask method.
type CancelTaskRequest struct {
	Tenant   string `json:"tenant,omitempty"`
	ID       string `json:"id"`
	Metadata any    `json:"metadata,omitempty"`
}

// SubscribeToTaskRequest is the params object of the SubscribeToTask method.
type SubscribeToTaskRequest struct {
	Tenant string `json:"tenant,omitempty"`
	ID     string `json:"id"`
}

// ListTasksRequest is the params object of the ListTasks method.
type ListTasksRequest struct {
	Tenant               string `json:"tenant,omitempty"`
	ContextID            string `json:"contextId,omitempty"`
	Status               string `json:"status,omitempty"`
	PageSize             *int   `json:"pageSize,omitempty"`
	PageToken            string `json:"pageToken,omitempty"`
	HistoryLength        *int   `json:"historyLength,omitempty"`
	StatusTimestampAfter string `json:"statusTimestampAfter,omitempty"`
	IncludeArtifacts     bool   `json:"includeArtifacts,omitempty"`
}

// ListTasksResponse is the result object of the ListTasks method.
type ListTasksResponse struct {
	Tasks         []Task `json:"tasks"`
	NextPageToken string `json:"nextPageToken"`
	PageSize      int    `json:"pageSize"`
	TotalSize     int    `json:"totalSize"`
}

// PushNotificationConfig is a webhook registered for task updates.
type PushNotificationConfig struct {
	ID             string              `json:"id,omitempty"`
	TaskID         string              `json:"taskId,omitempty"`
	URL            string              `json:"url"`
	Token          string              `json:"token,omitempty"`
	Authentication *AuthenticationInfo `json:"authentication,omitempty"`
}

// AuthenticationInfo defines how the webhook is authenticated.
type AuthenticationInfo struct {
	Scheme      string `json:"scheme"`
	Credentials string `json:"credentials,omitempty"`
}

// GetTaskPushNotificationConfigRequest is the params of
// GetTaskPushNotificationConfig.
type GetTaskPushNotificationConfigRequest struct {
	Tenant string `json:"tenant,omitempty"`
	TaskID string `json:"taskId"`
	ID     string `json:"id"`
}

// ListTaskPushNotificationConfigsRequest is the params of
// ListTaskPushNotificationConfigs.
type ListTaskPushNotificationConfigsRequest struct {
	Tenant    string `json:"tenant,omitempty"`
	TaskID    string `json:"taskId"`
	PageSize  *int   `json:"pageSize,omitempty"`
	PageToken string `json:"pageToken,omitempty"`
}

// ListTaskPushNotificationConfigsResponse is the result of
// ListTaskPushNotificationConfigs.
type ListTaskPushNotificationConfigsResponse struct {
	Configs       []PushNotificationConfig `json:"configs"`
	NextPageToken string                   `json:"nextPageToken,omitempty"`
}

// DeleteTaskPushNotificationConfigRequest is the params of
// DeleteTaskPushNotificationConfig.
type DeleteTaskPushNotificationConfigRequest struct {
	Tenant string `json:"tenant,omitempty"`
	TaskID string `json:"taskId"`
	ID     string `json:"id"`
}

// GetExtendedAgentCardRequest is the params of GetExtendedAgentCard.
type GetExtendedAgentCardRequest struct {
	Tenant string `json:"tenant,omitempty"`
}

// StreamResponse is the oneof wrapper used by streaming operations and
// push notification payloads.
type StreamResponse struct {
	Task           *Task                    `json:"task,omitempty"`
	Message        *Message                 `json:"message,omitempty"`
	StatusUpdate   *TaskStatusUpdateEvent   `json:"statusUpdate,omitempty"`
	ArtifactUpdate *TaskArtifactUpdateEvent `json:"artifactUpdate,omitempty"`
}

// TaskStatusUpdateEvent notifies the client of a task status change.
type TaskStatusUpdateEvent struct {
	TaskID    string     `json:"taskId"`
	ContextID string     `json:"contextId,omitempty"`
	Status    TaskStatus `json:"status"`
	Metadata  any        `json:"metadata,omitempty"`
}

// TaskArtifactUpdateEvent notifies the client of an artifact update.
type TaskArtifactUpdateEvent struct {
	TaskID    string   `json:"taskId"`
	ContextID string   `json:"contextId,omitempty"`
	Artifact  Artifact `json:"artifact"`
	Append    bool     `json:"append,omitempty"`
	LastChunk bool     `json:"lastChunk,omitempty"`
	Metadata  any      `json:"metadata,omitempty"`
}

// String returns a readable representation for logs.
func (e TaskStatusUpdateEvent) String() string {
	return fmt.Sprintf("status %s for %s", e.Status.State, e.TaskID)
}
