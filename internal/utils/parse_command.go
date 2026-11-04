package utils

import "strings"

// ParseCommand splits a slash command into its name and arguments.
//
// Telegram appends the bot username to commands sent in groups
// (/start@MyaawBot), so the mention suffix is stripped here to keep command
// dispatch working in chats with more than one bot.
func ParseCommand(commandText string) (bool, string, string) {
	if len(commandText) == 0 || commandText[0] != '/' {
		return false, "", ""
	}

	command := commandText[1:]

	parts := strings.SplitN(command, " ", 2)
	command = parts[0]
	if at := strings.Index(command, "@"); at > 0 {
		command = command[:at]
	}
	var commandArgs string
	if len(parts) > 1 {
		commandArgs = strings.TrimSpace(parts[1])
	}

	return true, command, commandArgs
}
