package common

import "myaaw/internal/agent"

func RoleSystemDefault() string {
	return agent.GetDefaultPersona()
}

func CommandStart() string {
	return "👋 Welcome! I’m Myaaw your personal assistant.\nHere are some commands to configure me:\n\n" +
		"**/about** - Info about Myaaw project\n" +
		"**/me** - About me and show current config\n\n" +
		"**/models** - Change the LLM model\n\n" +
		"**/reset** - Reset the history context windows\n\n" +
		"ℹ️ You can interact using natural language without needing to set commands first."
}

func CommandAbout() string {
	return "📣 Feel free to contribute to the project!\nhttps://github.com/Shiyinq/myaaw"
}

func CommandReset() string {
	return "✅ History and context window have been reset."
}

func CommandResetFailed() string {
	return "❌ Failed to reset history and context window. Please try again later."
}

func CommandSystem() string {
	return "✅ System prompt has been updated successfully."
}

func CommandSystemNeedArgs() string {
	return "⚠️ Please provide a prompt after the command.\nExample:\n/system You are a helpful assistant."
}

func CommandSystemFailed() string {
	return "❌ Failed to update the system prompt. Please try again later."
}

func CommandNotFound() string {
	return "4️⃣0️⃣4️⃣ Command not found."
}

func CommandModels() string {
	return "✅ Model has been updated successfully."
}

func CommandModelsFailed() string {
	return "❌ Failed to show models. Please try again later."
}

func CommandModelsArgsNotInt() string {
	return "⚠️ The model ID must be an integer. Example: /models 2"
}

func CommandModelsNotFound() string {
	return "4️⃣0️⃣4️⃣ Model not found"
}

func CommandModelsUpdateFailed() string {
	return "❌ Failed to update the model. Please try again later."
}
