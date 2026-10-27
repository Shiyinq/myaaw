package contact

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
	"myaaw/internal/services/a2a"

	"github.com/google/uuid"
)

const (
	maxResponseBytes = 16 << 20
	maxCardSkills    = 20

	// maxArtifactDownloadBytes bounds downloading a `url` file artifact.
	maxArtifactDownloadBytes = 1 << 30

	// maxRPCAttempts bounds retries of one JSON-RPC call. Retries reuse the
	// same request body, so the remote side sees the same messageId and can
	// deduplicate instead of running the task twice.
	maxRPCAttempts = 3
	rpcRetryDelay  = 300 * time.Millisecond

	actionSend   = "send"
	actionStatus = "status"
	actionList   = "list"
	actionGet    = "get"
	actionAdd    = "add"
)

type contactEntry struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	Description string `json:"description,omitempty"`
}

// ContactTool manages and calls agents registered in contacts.json.
type ContactTool struct {
	client     *http.Client
	cardClient *http.Client
}

func init() {
	tools.RegisterBuiltin("contact", NewContactTool())
}

func NewContactTool() *ContactTool {
	return &ContactTool{
		// No client timeout: per-request contexts carry the configured budget,
		// which is read at call time because the tool is built during init.
		client:     &http.Client{},
		cardClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// requestTimeout bounds one call to a contact. The default (16 minutes) is
// deliberately longer than the remote agent's default run timeout (15
// minutes) so the caller does not abandon a task that is still running.
func requestTimeout() time.Duration {
	seconds := config.A2AContactTimeoutSeconds
	if seconds <= 0 {
		seconds = 960
	}
	return time.Duration(seconds) * time.Second
}

func (t *ContactTool) ToolDefinition() []byte {
	return []byte(`{
		"type": "function",
		"function": {
			"name": "contact",
			"description": "Work with trusted contact agents over the A2A protocol. action=send delegates a task and returns the answer; set async=true to return immediately with a taskId, then use action=status to poll it. Pass taskId to action=send to answer a task that returned [input-required taskId=...]. action=list lists contacts; action=get shows one contact and its capabilities; action=add registers a new contact (only when the user explicitly provides the URL). Aliases accepted: send_message, list_contacts, get_contact, add_contact, poll.",
			"parameters": {
				"type": "object",
				"properties": {
					"action": {
						"type": "string",
						"enum": ["send", "status", "list", "get", "add"],
						"description": "Operation to perform"
					},
					"async": {
						"type": "boolean",
						"description": "For send: return immediately with a taskId instead of waiting for completion. Use action=status to poll it later"
					},
					"contact": {
						"type": "string",
						"description": "Contact agent name. Required for send, get, and add"
					},
					"message": {
						"type": "string",
						"description": "Task or message to send. Required for send"
					},
					"taskId": {
						"type": "string",
						"description": "Task ID from a previous [input-required taskId=...] or [async taskId=...] result"
					},
					"url": {
						"type": "string",
						"description": "A2A endpoint URL for add, for example http://host:8081/a2a"
					},
					"token": {
						"type": "string",
						"description": "Optional bearer token for add. Never echoed back"
					},
					"description": {
						"type": "string",
						"description": "Optional human-readable description for add"
					}
				},
				"required": ["action"]
			}
		}
	}`)
}

type contactArgs struct {
	Action      string `json:"action"`
	Contact     string `json:"contact"`
	Message     string `json:"message"`
	TaskID      string `json:"taskId"`
	URL         string `json:"url"`
	Token       string `json:"token"`
	Description string `json:"description"`
	Async       bool   `json:"async"`
}

func (t *ContactTool) CallTool(arguments string, _ *tools.ToolsContext) string {
	var args contactArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	rawAction := strings.TrimSpace(args.Action)
	action := normalizeAction(rawAction)
	if action == "" {
		if rawAction != "" {
			return fmt.Sprintf("Error: unknown contact action %q. Available actions: send, status, list, get, add", rawAction)
		}
		// Backwards compatible with calls made before the action field existed.
		if strings.TrimSpace(args.Contact) != "" && strings.TrimSpace(args.Message) != "" {
			action = actionSend
		} else {
			return "Error: action is required. Available actions: send, status, list, get, add"
		}
	}

	switch action {
	case actionSend:
		return t.send(args)
	case actionStatus:
		return t.status(args)
	case actionList:
		return t.list()
	case actionGet:
		return t.get(args.Contact)
	case actionAdd:
		return t.add(args)
	default:
		return "Error: action is required. Available actions: send, status, list, get, add"
	}
}

func (t *ContactTool) send(args contactArgs) string {
	contactName := strings.TrimSpace(args.Contact)
	message := strings.TrimSpace(args.Message)
	taskID := strings.TrimSpace(args.TaskID)
	if contactName == "" || message == "" {
		return "Error: contact and message are required for action send"
	}

	contacts, err := loadContacts()
	if err != nil {
		return fmt.Sprintf("Error loading contacts: %v", err)
	}
	target := findContact(contacts, contactName)
	if target == nil {
		return notFoundMessage(contactName, contacts)
	}

	task, err := t.sendTask(target, message, taskID, args.Async)
	if err != nil {
		var callErr *rpcCallError
		if errors.As(err, &callErr) && callErr.Code == -32004 {
			return fmt.Sprintf("Error: task %q is not waiting for input. Send without taskId to start a new task.", taskID)
		}
		return fmt.Sprintf("Error contacting %s: %v", target.Name, err)
	}

	if args.Async {
		switch a2a.StateFromWire(task.Status.State) {
		case a2a.StateSubmitted, a2a.StateWorking:
			return asyncMessage(task)
		}
	}
	return t.renderTask(target, task, taskID)
}

func asyncMessage(task rpcTask) string {
	return fmt.Sprintf("[async taskId=%s] state=%s\nTask accepted. Use contact action \"status\" with this taskId to check progress.", task.ID, a2a.WireState(task.Status.State))
}

// status polls a previously started task and renders its result once it is done.
func (t *ContactTool) status(args contactArgs) string {
	contactName := strings.TrimSpace(args.Contact)
	taskID := strings.TrimSpace(args.TaskID)
	if contactName == "" || taskID == "" {
		return "Error: contact and taskId are required for action status"
	}

	contacts, err := loadContacts()
	if err != nil {
		return fmt.Sprintf("Error loading contacts: %v", err)
	}
	target := findContact(contacts, contactName)
	if target == nil {
		return notFoundMessage(contactName, contacts)
	}

	task, err := t.getTask(target, taskID)
	if err != nil {
		return fmt.Sprintf("Error contacting %s: %v", target.Name, err)
	}
	switch a2a.StateFromWire(task.Status.State) {
	case a2a.StateSubmitted, a2a.StateWorking:
		return fmt.Sprintf("[async taskId=%s] state=%s\nTask is still running. Poll again with action \"status\".", taskID, a2a.WireState(task.Status.State))
	}
	return t.renderTask(target, task, taskID)
}

// renderTask renders a terminal or interrupted task as the tool result.
func (t *ContactTool) renderTask(contact *contactEntry, task rpcTask, fallbackID string) string {
	switch a2a.StateFromWire(task.Status.State) {
	case a2a.StateFailed:
		if msg := messageText(task.Status.Message); msg != "" {
			return fmt.Sprintf("Contact %s failed: %s", contact.Name, msg)
		}
		return fmt.Sprintf("Contact %s failed", contact.Name)
	case a2a.StateCanceled:
		return fmt.Sprintf("Contact %s canceled the task", contact.Name)
	case a2a.StateSubmitted, a2a.StateWorking:
		id := task.ID
		if id == "" {
			id = fallbackID
		}
		return fmt.Sprintf("[async taskId=%s] state=%s\nTask is still running. Poll again with action \"status\".", id, a2a.WireState(task.Status.State))
	case a2a.StateInputRequired, a2a.StateAuthRequired:
		question := messageText(task.Status.Message)
		if question == "" {
			question = "The agent needs more information to continue."
		}
		id := task.ID
		if id == "" {
			id = fallbackID
		}
		return fmt.Sprintf("[input-required taskId=%s]\n%s", id, question)
	}

	parts := make([]string, 0)
	if text := artifactText(task.Artifacts); text != "" {
		parts = append(parts, text)
	}
	if files := t.saveArtifacts(contact, task); files != "" {
		parts = append(parts, files)
	}
	if len(parts) == 0 {
		if text := agentText(task); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "Contact completed without output"
	}
	return strings.Join(parts, "\n\n")
}

func (t *ContactTool) list() string {
	contacts, err := loadContacts()
	if err != nil {
		return fmt.Sprintf("Error loading contacts: %v", err)
	}
	if len(contacts) == 0 {
		return "No contacts configured. Use action add to register one."
	}

	sorted := append([]contactEntry(nil), contacts...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})

	var b strings.Builder
	fmt.Fprintf(&b, "Contacts (%d):\n", len(sorted))
	for _, contact := range sorted {
		fmt.Fprintf(&b, "- %s", contact.Name)
		if contact.Description != "" {
			fmt.Fprintf(&b, ": %s", contact.Description)
		}
		fmt.Fprintf(&b, " (%s)", contact.URL)
		if contact.Token != "" {
			b.WriteString(" [token configured]")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (t *ContactTool) get(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Error: contact is required for action get"
	}

	contacts, err := loadContacts()
	if err != nil {
		return fmt.Sprintf("Error loading contacts: %v", err)
	}
	target := findContact(contacts, name)
	if target == nil {
		return notFoundMessage(name, contacts)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Contact: %s\n", target.Name)
	fmt.Fprintf(&b, "URL: %s\n", target.URL)
	if target.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", target.Description)
	}
	if target.Token != "" {
		b.WriteString("Token: configured\n")
	} else {
		b.WriteString("Token: none\n")
	}

	card, err := t.fetchCard(target)
	if err != nil {
		fmt.Fprintf(&b, "Agent card: unavailable (%v)\n", err)
		return strings.TrimRight(b.String(), "\n")
	}
	fmt.Fprintf(&b, "Agent card: %s", card.Name)
	if card.Description != "" {
		fmt.Fprintf(&b, " - %s", card.Description)
	}
	b.WriteString("\n")
	if len(card.Signatures) > 0 {
		if kid, err := a2a.VerifyCard(card, a2a.ParseTrustedCardKeys(config.A2ATrustedKeys)); err == nil {
			fmt.Fprintf(&b, "Card signature: verified (%s)\n", kid)
		} else {
			fmt.Fprintf(&b, "Card signature: unverified (%v)\n", err)
		}
	}
	if len(card.Skills) > 0 {
		skills := make([]string, 0, len(card.Skills))
		for i, skill := range card.Skills {
			if i >= maxCardSkills {
				skills = append(skills, fmt.Sprintf("+%d more", len(card.Skills)-maxCardSkills))
				break
			}
			if skill.Name != "" {
				skills = append(skills, fmt.Sprintf("%s (%s)", skill.ID, skill.Name))
			} else {
				skills = append(skills, skill.ID)
			}
		}
		fmt.Fprintf(&b, "Skills: %s\n", strings.Join(skills, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (t *ContactTool) add(args contactArgs) string {
	name := strings.TrimSpace(args.Contact)
	rawURL := strings.TrimSpace(args.URL)
	if err := validateContact(name, rawURL); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	contacts, err := loadContacts()
	if err != nil {
		return fmt.Sprintf("Error loading contacts: %v", err)
	}
	if existing := findContact(contacts, name); existing != nil {
		return fmt.Sprintf("Error: contact %q already exists (%s)", existing.Name, existing.URL)
	}

	entry := contactEntry{
		Name:        name,
		URL:         rawURL,
		Token:       strings.TrimSpace(args.Token),
		Description: strings.TrimSpace(args.Description),
	}
	contacts = append(contacts, entry)
	if err := saveContacts(contacts); err != nil {
		return fmt.Sprintf("Error saving contacts: %v", err)
	}

	tokenState := "none"
	if entry.Token != "" {
		tokenState = "configured"
	}
	return fmt.Sprintf("Added contact %q (%s). Token: %s", entry.Name, entry.URL, tokenState)
}

func normalizeAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case actionSend, "send_message", "delegate", "call":
		return actionSend
	case actionStatus, "poll", "task_status", "get_task", "check":
		return actionStatus
	case actionList, "list_contacts", "contacts", "ls":
		return actionList
	case actionGet, "get_contact", "inspect", "show":
		return actionGet
	case actionAdd, "add_contact", "create", "register":
		return actionAdd
	default:
		return ""
	}
}

// signRequest was removed with the legacy request-signing extension. The
// v1.0 client authenticates with a bearer token only.
func findContact(contacts []contactEntry, name string) *contactEntry {
	for i := range contacts {
		if strings.EqualFold(contacts[i].Name, name) {
			return &contacts[i]
		}
	}
	return nil
}

func notFoundMessage(name string, contacts []contactEntry) string {
	names := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		names = append(names, contact.Name)
	}
	if len(names) == 0 {
		return fmt.Sprintf("Error: contact %q not found. No contacts configured; use action add to register one.", name)
	}
	return fmt.Sprintf("Error: contact %q not found. Available contacts: %s", name, strings.Join(names, ", "))
}

func validateContact(name, rawURL string) error {
	if name == "" {
		return fmt.Errorf("contact name is required")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("invalid contact name %q", name)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("url must be an absolute http(s) URL")
	}
	return nil
}

func saveContacts(contacts []contactEntry) error {
	path, err := agentdir.ActiveContactsPath()
	if err != nil {
		return err
	}
	if contacts == nil {
		contacts = []contactEntry{}
	}
	data, err := json.MarshalIndent(contacts, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type agentCard = a2a.AgentCard

func (t *ContactTool) fetchCard(contact *contactEntry) (agentCard, error) {
	endpoint, err := agentCardURL(contact.URL)
	if err != nil {
		return agentCard{}, err
	}
	resp, err := t.cardClient.Get(endpoint)
	if err != nil {
		return agentCard{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return agentCard{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var card agentCard
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&card); err != nil {
		return agentCard{}, fmt.Errorf("invalid agent card: %w", err)
	}
	return card, nil
}

func agentCardURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid contact URL")
	}
	parsed.Path = "/.well-known/agent-card.json"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

type rpcTask struct {
	ID     string `json:"id"`
	Status struct {
		State   string      `json:"state"`
		Message *rpcMessage `json:"message,omitempty"`
	} `json:"status"`
	History   []rpcMessage  `json:"history"`
	Artifacts []rpcArtifact `json:"artifacts"`
}

// rpcCallError carries the JSON-RPC error code so callers can react to
// specific failures such as a task that is not resumable.
type rpcCallError struct {
	Code    int
	Message string
}

func (e *rpcCallError) Error() string {
	return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
}

type rpcPart struct {
	Text      string `json:"text"`
	Raw       string `json:"raw"`
	URL       string `json:"url"`
	Filename  string `json:"filename"`
	MediaType string `json:"mediaType"`
}

type rpcMessage struct {
	Role  string    `json:"role"`
	Parts []rpcPart `json:"parts"`
}

type rpcArtifact struct {
	Name  string    `json:"name"`
	Parts []rpcPart `json:"parts"`
}

// saveArtifacts persists file artifacts to the caller's downloads directory and
// returns a human-readable summary. `raw` parts are decoded inline; `url` parts
// are downloaded with the contact's bearer token.
func (t *ContactTool) saveArtifacts(contact *contactEntry, task rpcTask) string {
	home, err := agentdir.ActiveHome()
	if err != nil {
		return ""
	}
	destDir := filepath.Join(home, "workspace", "downloads", sanitizeDownloadName(task.ID))
	lines := make([]string, 0)
	for _, artifact := range task.Artifacts {
		for _, part := range artifact.Parts {
			name := sanitizeDownloadName(part.Filename)
			if name == "" {
				name = sanitizeDownloadName(filepath.Base(artifact.Name))
			}
			if name == "" {
				name = "file"
			}
			switch {
			case part.Raw != "":
				data, err := base64.StdEncoding.DecodeString(part.Raw)
				if err != nil {
					lines = append(lines, fmt.Sprintf("- %s: invalid base64 artifact", name))
					continue
				}
				if err := os.MkdirAll(destDir, 0o755); err != nil {
					lines = append(lines, fmt.Sprintf("- %s: %v", name, err))
					continue
				}
				saved := filepath.Join(destDir, name)
				if err := os.WriteFile(saved, data, 0o644); err != nil {
					lines = append(lines, fmt.Sprintf("- %s: %v", name, err))
					continue
				}
				lines = append(lines, fmt.Sprintf("- %s (%s, %s) saved at %s", name, humanSize(int64(len(data))), part.MediaType, saved))
			case part.URL != "":
				saved, size, err := t.downloadArtifact(contact, part.URL, destDir, name)
				if err != nil {
					lines = append(lines, fmt.Sprintf("- %s: download failed: %v", name, err))
					continue
				}
				lines = append(lines, fmt.Sprintf("- %s (%s, %s) saved at %s", name, humanSize(size), part.MediaType, saved))
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Files:\n" + strings.Join(lines, "\n")
}

func (t *ContactTool) downloadArtifact(contact *contactEntry, rawURL, destDir, name string) (string, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	if contact.Token != "" {
		req.Header.Set("Authorization", "Bearer "+contact.Token)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", 0, err
	}
	tmp := filepath.Join(destDir, name+".part")
	file, err := os.Create(tmp)
	if err != nil {
		return "", 0, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(resp.Body, maxArtifactDownloadBytes))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", 0, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", 0, closeErr
	}
	final := filepath.Join(destDir, name)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", 0, err
	}
	return final, written, nil
}

func sanitizeDownloadName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "/" || name == "" {
		return ""
	}
	replacer := strings.NewReplacer("..", "_", "/", "_", "\\", "_", "\x00", "_")
	return replacer.Replace(name)
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (t *ContactTool) sendTask(contact *contactEntry, message, taskID string, async bool) (rpcTask, error) {
	payload := buildSendPayload(message, taskID, async)
	result, err := t.rpc(contact, payload)
	if err != nil && !async && isTimeout(err) {
		// The blocking call outlived this client's budget while the remote task
		// kept running. Re-send the same messageId with returnImmediately so the
		// remote deduplicates and hands back the existing task instead of
		// starting a second run.
		withReturnImmediately(payload)
		if recovered, recoverErr := t.rpc(contact, payload); recoverErr == nil {
			result, err = recovered, nil
		}
	}
	if err != nil {
		return rpcTask{}, err
	}
	var envelope struct {
		Task    *rpcTask    `json:"task"`
		Message *rpcMessage `json:"message"`
	}
	if err := json.Unmarshal(result, &envelope); err != nil {
		return rpcTask{}, fmt.Errorf("invalid task response: %w", err)
	}
	if envelope.Task == nil {
		return rpcTask{}, fmt.Errorf("response is missing a task")
	}
	return *envelope.Task, nil
}

// withReturnImmediately switches an existing SendMessage payload to async mode
// without touching its messageId.
func withReturnImmediately(payload map[string]any) {
	params, ok := payload["params"].(map[string]any)
	if !ok {
		return
	}
	params["configuration"] = map[string]any{"returnImmediately": true}
}

// getTask polls a task with GetTask; unlike SendMessage its result is the task itself.
func (t *ContactTool) getTask(contact *contactEntry, taskID string) (rpcTask, error) {
	result, err := t.rpc(contact, buildGetTaskPayload(taskID))
	if err != nil {
		return rpcTask{}, err
	}
	var task rpcTask
	if err := json.Unmarshal(result, &task); err != nil {
		return rpcTask{}, fmt.Errorf("invalid task response: %w", err)
	}
	return task, nil
}

func buildSendPayload(message, taskID string, async bool) map[string]any {
	msg := map[string]any{
		"messageId": uuid.NewString(),
		"role":      "ROLE_USER",
		"parts":     []map[string]any{{"text": message}},
	}
	if taskID != "" {
		msg["taskId"] = taskID
	}
	params := map[string]any{"message": msg}
	if async {
		params["configuration"] = map[string]any{"returnImmediately": true}
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "SendMessage",
		"params":  params,
	}
}

func buildGetTaskPayload(taskID string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "GetTask",
		"params":  map[string]any{"id": taskID},
	}
}

// rpc performs a JSON-RPC call and returns the raw result payload. Transient
// failures (connection errors, 429, 5xx) are retried with the same body so an
// idempotent messageId reaches the remote side again. Timeouts are not retried
// here: sendTask recovers the running task instead of waiting another budget.
func (t *ContactTool) rpc(contact *contactEntry, payload map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= maxRPCAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(rpcRetryDelay * time.Duration(attempt-1))
		}
		result, retryable, err := t.rpcOnce(contact, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func (t *ContactTool) rpcOnce(contact *contactEntry, body []byte) (json.RawMessage, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contact.URL, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("A2A-Version", "1.0")
	if contact.Token != "" {
		req.Header.Set("Authorization", "Bearer "+contact.Token)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		if isTimeout(err) {
			return nil, false, err
		}
		return nil, true, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, true, err
	}
	if resp.StatusCode != http.StatusOK {
		callErr := fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		return nil, retryableStatus(resp.StatusCode), callErr
	}
	if len(data) == 0 {
		return nil, false, fmt.Errorf("empty response from %s (HTTP %d)", contact.URL, resp.StatusCode)
	}
	if len(data) > maxResponseBytes {
		return nil, false, fmt.Errorf("response from %s exceeded %d bytes; the remote task probably returned a large file artifact", contact.URL, maxResponseBytes)
	}

	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &rpc); err != nil {
		return nil, false, fmt.Errorf("invalid response from %s (HTTP %d, %d bytes): %w", contact.URL, resp.StatusCode, len(data), err)
	}
	if rpc.Error != nil {
		return nil, false, &rpcCallError{Code: rpc.Error.Code, Message: rpc.Error.Message}
	}
	if len(rpc.Result) == 0 {
		return nil, false, fmt.Errorf("response is missing a result")
	}
	return rpc.Result, false, nil
}

// isTimeout reports whether err is a client-side deadline or network timeout.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// retryableStatus reports whether an HTTP status is worth retrying with the
// same request body.
func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func agentText(task rpcTask) string {
	return messagesText(task.History)
}

func loadContacts() ([]contactEntry, error) {
	path, err := agentdir.ActiveContactsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var contacts []contactEntry
	if err := json.Unmarshal(data, &contacts); err != nil {
		return nil, err
	}
	return contacts, nil
}

func artifactText(artifacts []rpcArtifact) string {
	parts := make([]string, 0)
	for _, artifact := range artifacts {
		for _, part := range artifact.Parts {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func messagesText(messages []rpcMessage) string {
	parts := make([]string, 0)
	for _, message := range messages {
		if !strings.EqualFold(message.Role, "agent") && !strings.EqualFold(message.Role, "ROLE_AGENT") {
			continue
		}
		for _, part := range message.Parts {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func messageText(message *rpcMessage) string {
	if message == nil {
		return ""
	}
	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
