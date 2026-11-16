package utils

import "strings"

// ParseCommand splits a slash or bang command into its name and arguments.
//
// Both "/" and "!" start a command, with or without a bot mention. Telegram
// sometimes appends the bot's own username to a command sent in a group
// (/stop@YourBot); any suffix after "@" is stripped, so the same command works
// whatever the bot is named. Tagging the bot is never required.
//
// Chat platforms reserve "/" for their own slash-command UI, so "!stop" is the
// plain-text spelling a user can always send to reach the bot.
func ParseCommand(commandText string) (bool, string, string) {
	if CommandPrefix(commandText) == 0 {
		return false, "", ""
	}

	command := commandText[1:]

	parts := strings.SplitN(command, " ", 2)
	command = parts[0]
	if at := strings.Index(command, "@"); at > 0 {
		command = command[:at]
	}
	// Commands are case-insensitive: /Stop and !STOP mean the same as /stop.
	command = strings.ToLower(command)
	var commandArgs string
	if len(parts) > 1 {
		commandArgs = strings.TrimSpace(parts[1])
	}

	return true, command, commandArgs
}

// CommandPrefix reports the byte that starts a command, or 0 when the text is
// not a command. It lets callers tell the two prefixes apart: "/" always means
// a command, while "!" is a convenience prefix that is only a command when the
// name is one the bot knows.
func CommandPrefix(text string) byte {
	if text == "" {
		return 0
	}
	switch text[0] {
	case '/', '!':
		return text[0]
	default:
		return 0
	}
}
