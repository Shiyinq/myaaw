package common

func CommandStart() string {
	return "👋 Welcome! I’m Myaaw your personal assistant.\nHere are some commands to configure me:\n\n" +
		"**/about** - Info about Myaaw project\n" +
		"**/me** - About me and show current config\n\n" +
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

func CommandNotFound() string {
	return "4️⃣0️⃣4️⃣ Command not found."
}
