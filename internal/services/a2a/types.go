// Package a2a implements the server side of the Agent2Agent protocol for one
// Myaaw agent: discovery through an Agent Card and synchronous task endpoints
// over JSON-RPC 2.0.
package a2a

import "encoding/json"

// Task states used by Fase 1.
const (
	StateSubmitted     = "submitted"
	StateWorking       = "working"
	StateInputRequired = "input-required"
	StateCompleted     = "completed"
	StateFailed        = "failed"
	StateCanceled      = "canceled"
)

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
// /.well-known/agent.json. It never contains credentials.
type AgentCard struct {
	Name               string           `json:"name"`
	Description        string           `json:"description,omitempty"`
	URL                string           `json:"url"`
	Version            string           `json:"version"`
	Capabilities       Capabilities     `json:"capabilities"`
	DefaultInputModes  []string         `json:"defaultInputModes"`
	DefaultOutputModes []string         `json:"defaultOutputModes"`
	Skills             []AgentSkill     `json:"skills"`
	SecuritySchemes    []SecurityScheme `json:"securitySchemes,omitempty"`
	PublicKey          string           `json:"publicKey,omitempty"`
	KeyID              string           `json:"keyId,omitempty"`
	Signatures         []CardSignature  `json:"signatures,omitempty"`
}

// CardSignature is a compact JWS over the canonical Agent Card document.
type CardSignature struct {
	Protected string `json:"protected"`
	Signature string `json:"signature"`
}

type Capabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
}

type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type SecurityScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme,omitempty"`
}

// Task is the unit of work exchanged between agents.
type Task struct {
	Kind      string     `json:"kind,omitempty"`
	ID        string     `json:"id"`
	ContextID string     `json:"contextId,omitempty"`
	Status    TaskStatus `json:"status"`
	Messages  []Message  `json:"messages,omitempty"`
	History   []Message  `json:"history,omitempty"`
	Artifacts []Artifact `json:"artifacts,omitempty"`

	// Internal fields, never exposed to callers.
	Attempts int    `json:"-"`
	PushURL  string `json:"-"`
	Caller   string `json:"-"`
}

// ForResponse returns a copy shaped for the wire: it carries the task kind and
// exposes the transcript as history alongside the legacy messages field.
func (t Task) ForResponse() Task {
	t.Kind = "task"
	t.History = t.Messages
	return t
}

type TaskStatus struct {
	State     string   `json:"state"`
	Timestamp string   `json:"timestamp,omitempty"`
	Message   *Message `json:"message,omitempty"`
}

type Message struct {
	Kind      string `json:"kind,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Role      string `json:"role"`
	Parts     []Part `json:"parts"`

	// ContextID and TaskID are accepted inside incoming messages by newer A2A
	// clients. They are never set on stored agent messages.
	ContextID string `json:"contextId,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
}

// Part is one piece of a message or artifact. Fase 1 uses text parts.
type Part struct {
	Kind string    `json:"kind"`
	Text string    `json:"text,omitempty"`
	Data any       `json:"data,omitempty"`
	File *FilePart `json:"file,omitempty"`
}

type FilePart struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Bytes    string `json:"bytes,omitempty"` // base64
	URI      string `json:"uri,omitempty"`
}

type Artifact struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parts       []Part `json:"parts"`
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
	Data    any    `json:"data,omitempty"`
}

// Standard and A2A-specific JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
	CodeTaskNotFound   = -32001
	CodeTaskExists     = -32002
	CodeForbidden      = -32003
)

type SendTaskParams struct {
	ID                  string  `json:"id,omitempty"`
	ContextID           string  `json:"contextId,omitempty"`
	Message             Message `json:"message"`
	Async               bool    `json:"async,omitempty"`
	PushNotificationURL string  `json:"pushNotificationUrl,omitempty"`
	Caller              string  `json:"-"`
}

type TaskQueryParams struct {
	ID string `json:"id"`
}

// PushNotificationConfig is a webhook registered for task updates.
type PushNotificationConfig struct {
	ID    string `json:"id,omitempty"`
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

type SetPushConfigParams struct {
	ID                     string                 `json:"id"`
	PushNotificationConfig PushNotificationConfig `json:"pushNotificationConfig"`
}

type PushConfigIDParams struct {
	ID                       string `json:"id"`
	PushNotificationConfigID string `json:"pushNotificationConfigId,omitempty"`
}

// StreamEvent is one SSE payload emitted by tasks/sendSubscribe.
type StreamEvent struct {
	Kind      string      `json:"kind"`
	TaskID    string      `json:"taskId,omitempty"`
	Status    *TaskStatus `json:"status,omitempty"`
	Artifact  *Artifact   `json:"artifact,omitempty"`
	Append    bool        `json:"append,omitempty"`
	LastChunk bool        `json:"lastChunk,omitempty"`
	Final     bool        `json:"final,omitempty"`
}
