package discord

// discordScope maps a Discord chat to a conversation scope.
//
// Guild channels and threads use their channel id, which Discord generates
// globally, so no guild prefix is needed. DMs and payloads without a chat type
// keep the legacy per-user session, which also keeps a rolling upgrade safe.
func discordScope(chatType, channelID string) string {
	if channelID == "" || chatType == "" || chatType == "dm" {
		return ""
	}
	return "dc:" + channelID
}
