package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myaaw/internal/agent/approvals"

	"github.com/go-resty/resty/v2"
)

// ApprovalRegistry tracks pending Telegram approval decisions.
var ApprovalRegistry = approvals.NewRegistry()

const approvalCallbackPrefix = "apv:"

// ApprovalRequester sends approval prompts to a Telegram chat and waits for a
// button press.
type ApprovalRequester struct {
	ChatID  int
	ReplyTo int
}

func NewApprovalRequester(chatID, replyTo int) *ApprovalRequester {
	return &ApprovalRequester{ChatID: chatID, ReplyTo: replyTo}
}

func (r *ApprovalRequester) Request(ctx context.Context, req approvals.Request) (approvals.Decision, error) {
	decisions := ApprovalRegistry.Register(req.ID)
	defer ApprovalRegistry.Remove(req.ID)

	messageID, err := r.send(req)
	if err != nil {
		return approvals.Deny, err
	}

	select {
	case decision := <-decisions:
		r.finish(messageID, req, decision, "")
		return decision, nil
	case <-ctx.Done():
		r.finish(messageID, req, approvals.Deny, "timed out")
		return approvals.Deny, ctx.Err()
	}
}

func (r *ApprovalRequester) send(req approvals.Request) (int, error) {
	target := req.ToolName
	if req.Action != "" {
		target += ":" + req.Action
	}
	text := fmt.Sprintf("🔐 Approval required: %s", target)
	if req.SubAgent != "" {
		text += fmt.Sprintf("\n↳ from sub-agent %s", req.SubAgent)
	}
	if req.ArgsSummary != "" {
		text += "\n" + req.ArgsSummary
	}

	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]string{
			{
				{"text": "Allow once", "callback_data": approvalCallbackPrefix + req.ID + ":y"},
				{"text": "Allow session", "callback_data": approvalCallbackPrefix + req.ID + ":s"},
			},
			{
				{"text": "Always", "callback_data": approvalCallbackPrefix + req.ID + ":a"},
				{"text": "Deny", "callback_data": approvalCallbackPrefix + req.ID + ":n"},
			},
		},
	}
	body := &TelegramSendMessage{
		Text:             text,
		ChatID:           r.ChatID,
		ReplyToMessageID: r.ReplyTo,
		ReplyMarkup:      keyboard,
	}
	status, err := SendTelegramRequest("sendMessage", body, r.ChatID)
	if err != nil {
		return 0, err
	}
	if status == nil || !status.Ok {
		return 0, fmt.Errorf("telegram sendMessage failed")
	}
	return status.Result.MessageId, nil
}

func (r *ApprovalRequester) finish(messageID int, req approvals.Request, decision approvals.Decision, reason string) {
	if messageID == 0 {
		return
	}
	target := req.ToolName
	if req.Action != "" {
		target += ":" + req.Action
	}
	text := fmt.Sprintf("🔐 %s — %s", target, decisionLabel(decision))
	if reason != "" {
		text += " (" + reason + ")"
	}
	_, _ = EditTelegramMessage(r.ChatID, 0, messageID, text, "")
}

func decisionLabel(decision approvals.Decision) string {
	switch decision {
	case approvals.AllowOnce:
		return "allowed once"
	case approvals.AllowSession:
		return "allowed for this session"
	case approvals.AllowAlways:
		return "always allowed"
	default:
		return "denied"
	}
}

type telegramCallbackQuery struct {
	ID      string `json:"id"`
	Data    string `json:"data"`
	Message *struct {
		MessageID int `json:"message_id"`
		Chat      struct {
			ID int `json:"id"`
		} `json:"chat"`
	} `json:"message"`
	From *struct {
		ID int `json:"id"`
	} `json:"from"`
}

// HandleApprovalCallback resolves an approval button press. It returns true
// when the update was a callback query (and therefore must not be enqueued as a
// chat message).
func HandleApprovalCallback(payload json.RawMessage) bool {
	var update struct {
		CallbackQuery *telegramCallbackQuery `json:"callback_query"`
	}
	if err := json.Unmarshal(payload, &update); err != nil || update.CallbackQuery == nil {
		return false
	}

	cb := update.CallbackQuery
	if id, decision, ok := parseApprovalCallback(cb.Data); ok {
		ApprovalRegistry.Resolve(id, decision)
	}
	answerCallbackQuery(cb.ID)
	return true
}

func parseApprovalCallback(data string) (string, approvals.Decision, bool) {
	if !strings.HasPrefix(data, approvalCallbackPrefix) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(data, approvalCallbackPrefix), ":", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	switch parts[1] {
	case "y":
		return parts[0], approvals.AllowOnce, true
	case "s":
		return parts[0], approvals.AllowSession, true
	case "a":
		return parts[0], approvals.AllowAlways, true
	case "n":
		return parts[0], approvals.Deny, true
	}
	return "", "", false
}

func answerCallbackQuery(callbackID string) {
	client := resty.New().SetTimeout(10 * time.Second)
	_, _ = client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{"callback_query_id": callbackID}).
		Post(telegramAPIURL("answerCallbackQuery"))
}
