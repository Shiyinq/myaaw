package common

import "teo/internal/config"

func RoleSystemDefault() string {
	return "You are Teo, a helpful assistant living in Telegram. Respond to users using Telegram's supported MarkdownV2 style."
}

func ModelDefault() string {
	return config.OllamaDefaultModel
}

func CommandStart() string {
	return "Welcome! I’m Teo your personal assistant.\nHere are some commands to configure me:\n\n- **/start** - Welcome message and menu display\n- **/system <prompt>** - Set the system prompt\n- **/models** - Change the LLM model\n- **/reset** - Reset the history context windows\n- **/about** - Info about Teo project\n\nYou can interact using natural language without needing to set commands first."
}

func CommandAbout() string {
	return "Feel free to contribute to the project!\nhttps://github.com/Shiyinq/teo"
}

func CommandReset() string {
	return "History and context window have been reset."
}

func CommandResetFailed() string {
	return "Failed to reset history and context window. Please try again later."
}

func CommandSystem() string {
	return "System prompt has been updated successfully."
}

func CommandSystemNeedArgs() string {
	return "Please provide a prompt after the command.\nExample:\n/system You are a helpful assistant."
}

func CommandSystemFailed() string {
	return "Failed to update the system prompt. Please try again later."
}
