package discord

import (
	"context"
	"fmt"
	"strings"

	"myaaw/internal/agent/approvals"

	"github.com/bwmarrin/discordgo"
)

// ApprovalRegistry tracks pending Discord approval decisions.
var ApprovalRegistry = approvals.NewRegistry()

const approvalCustomIDPrefix = "apv:"

// ApprovalRequester sends approval prompts to a Discord channel and waits for a
// button press.
type ApprovalRequester struct {
	session   *discordgo.Session
	channelID string
}

func (d *DiscordAdapter) NewApprovalRequester(channelID string) *ApprovalRequester {
	return &ApprovalRequester{session: d.session, channelID: channelID}
}

func (r *ApprovalRequester) Request(ctx context.Context, req approvals.Request) (approvals.Decision, error) {
	if r.session == nil {
		return approvals.Deny, fmt.Errorf("discord session is not available")
	}
	decisions := ApprovalRegistry.Register(req.ID)
	defer ApprovalRegistry.Remove(req.ID)

	target := req.ToolName
	if req.Action != "" {
		target += ":" + req.Action
	}
	content := fmt.Sprintf("🔐 Approval required: %s", target)
	if req.SubAgent != "" {
		content += fmt.Sprintf("\n↳ from sub-agent %s", req.SubAgent)
	}
	if req.ArgsSummary != "" {
		content += "\n" + req.ArgsSummary
	}

	msg, err := r.session.ChannelMessageSendComplex(r.channelID, &discordgo.MessageSend{
		Content: content,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Allow once",
					Style:    discordgo.PrimaryButton,
					CustomID: approvalCustomIDPrefix + req.ID + ":y",
				},
				discordgo.Button{
					Label:    "Allow session",
					Style:    discordgo.SecondaryButton,
					CustomID: approvalCustomIDPrefix + req.ID + ":s",
				},
				discordgo.Button{
					Label:    "Always",
					Style:    discordgo.SecondaryButton,
					CustomID: approvalCustomIDPrefix + req.ID + ":a",
				},
				discordgo.Button{
					Label:    "Deny",
					Style:    discordgo.DangerButton,
					CustomID: approvalCustomIDPrefix + req.ID + ":n",
				},
			}},
		},
	})
	if err != nil {
		return approvals.Deny, err
	}

	select {
	case decision := <-decisions:
		r.finish(msg.ID, target, decision, "")
		return decision, nil
	case <-ctx.Done():
		r.finish(msg.ID, target, approvals.Deny, "timed out")
		return approvals.Deny, ctx.Err()
	}
}

func (r *ApprovalRequester) finish(messageID, target string, decision approvals.Decision, reason string) {
	content := fmt.Sprintf("🔐 %s — %s", target, decisionLabel(decision))
	if reason != "" {
		content += " (" + reason + ")"
	}
	empty := []discordgo.MessageComponent{}
	_, _ = r.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    r.channelID,
		ID:         messageID,
		Content:    &content,
		Components: &empty,
	})
}

func handleApprovalInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}
	data := i.MessageComponentData()
	id, decision, ok := parseApprovalCustomID(data.CustomID)
	if !ok {
		return
	}
	ApprovalRegistry.Resolve(id, decision)

	content := "🔐 Approval: " + decisionLabel(decision)
	empty := []discordgo.MessageComponent{}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Components: empty,
		},
	})
}

func parseApprovalCustomID(customID string) (string, approvals.Decision, bool) {
	if !strings.HasPrefix(customID, approvalCustomIDPrefix) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(customID, approvalCustomIDPrefix), ":", 2)
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
