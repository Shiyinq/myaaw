package contact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	maxResponseBytes = 2 << 20
	maxCardSkills    = 20

	actionSend = "send"
	actionList = "list"
	actionGet  = "get"
	actionAdd  = "add"
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
		client:     &http.Client{Timeout: 10 * time.Minute},
		cardClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (t *ContactTool) ToolDefinition() []byte {
	return []byte(`{
		"type": "function",
		"function": {
			"name": "contact",
			"description": "Work with trusted contact agents over the A2A protocol. action=send delegates a task and returns the answer; pass taskId to answer a task that returned [input-required taskId=...]. action=list lists contacts; action=get shows one contact and its capabilities; action=add registers a new contact (only when the user explicitly provides the URL). Aliases accepted: send_message, list_contacts, get_contact, add_contact.",
			"parameters": {
				"type": "object",
				"properties": {
					"action": {
						"type": "string",
						"enum": ["send", "list", "get", "add"],
						"description": "Operation to perform"
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
						"description": "Task ID from a previous [input-required taskId=...] result, used to answer the agent's question"
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
			return fmt.Sprintf("Error: unknown contact action %q. Available actions: send, list, get, add", rawAction)
		}
		// Backwards compatible with calls made before the action field existed.
		if strings.TrimSpace(args.Contact) != "" && strings.TrimSpace(args.Message) != "" {
			action = actionSend
		} else {
			return "Error: action is required. Available actions: send, list, get, add"
		}
	}

	switch action {
	case actionSend:
		return t.send(args)
	case actionList:
		return t.list()
	case actionGet:
		return t.get(args.Contact)
	case actionAdd:
		return t.add(args)
	default:
		return "Error: action is required. Available actions: send, list, get, add"
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

	task, err := t.sendTask(target, message, taskID)
	if err != nil {
		var callErr *rpcCallError
		if errors.As(err, &callErr) && callErr.Code == -32004 {
			return fmt.Sprintf("Error: task %q is not waiting for input. Send without taskId to start a new task.", taskID)
		}
		return fmt.Sprintf("Error contacting %s: %v", target.Name, err)
	}

	switch a2a.StateFromWire(task.Status.State) {
	case a2a.StateFailed:
		if msg := messageText(task.Status.Message); msg != "" {
			return fmt.Sprintf("Contact %s failed: %s", target.Name, msg)
		}
		return fmt.Sprintf("Contact %s failed", target.Name)
	case a2a.StateCanceled:
		return fmt.Sprintf("Contact %s canceled the task", target.Name)
	case a2a.StateInputRequired, a2a.StateAuthRequired:
		question := messageText(task.Status.Message)
		if question == "" {
			question = "The agent needs more information to continue."
		}
		id := task.ID
		if id == "" {
			id = taskID
		}
		return fmt.Sprintf("[input-required taskId=%s]\n%s", id, question)
	}

	if text := artifactText(task.Artifacts); text != "" {
		return text
	}
	if text := agentText(task); text != "" {
		return text
	}
	return "Contact completed without output"
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

type rpcMessage struct {
	Role  string `json:"role"`
	Parts []struct {
		Text string `json:"text"`
	} `json:"parts"`
}

type rpcArtifact struct {
	Parts []struct {
		Text string `json:"text"`
	} `json:"parts"`
}

func (t *ContactTool) sendTask(contact *contactEntry, message, taskID string) (rpcTask, error) {
	return t.callRPC(contact, buildSendPayload(message, taskID))
}

func buildSendPayload(message, taskID string) map[string]any {
	msg := map[string]any{
		"messageId": uuid.NewString(),
		"role":      "ROLE_USER",
		"parts":     []map[string]any{{"text": message}},
	}
	if taskID != "" {
		msg["taskId"] = taskID
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "SendMessage",
		"params":  map[string]any{"message": msg},
	}
}

func (t *ContactTool) callRPC(contact *contactEntry, payload map[string]any) (rpcTask, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return rpcTask{}, err
	}

	req, err := http.NewRequest(http.MethodPost, contact.URL, bytes.NewReader(body))
	if err != nil {
		return rpcTask{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("A2A-Version", "1.0")
	if contact.Token != "" {
		req.Header.Set("Authorization", "Bearer "+contact.Token)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return rpcTask{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return rpcTask{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return rpcTask{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var rpc struct {
		Result *struct {
			Task    *rpcTask    `json:"task"`
			Message *rpcMessage `json:"message"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &rpc); err != nil {
		return rpcTask{}, fmt.Errorf("invalid response: %w", err)
	}
	if rpc.Error != nil {
		return rpcTask{}, &rpcCallError{Code: rpc.Error.Code, Message: rpc.Error.Message}
	}
	if rpc.Result == nil || rpc.Result.Task == nil {
		return rpcTask{}, fmt.Errorf("response is missing a task")
	}
	return *rpc.Result.Task, nil
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
