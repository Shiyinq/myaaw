package utils

import "strings"

func ParseCommand(commandText string) (bool, string, string) {
	if len(commandText) == 0 || commandText[0] != '/' {
		return false, "", ""
	}

	command := commandText[1:]

	parts := strings.SplitN(command, " ", 2)
	command = parts[0]
	var commandArgs string
	if len(parts) > 1 {
		commandArgs = strings.TrimSpace(parts[1])
	}

	return true, command, commandArgs
}
