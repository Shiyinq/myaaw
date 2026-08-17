package common

func CommandStart() string {
	return "👋 Welcome! I’m Myaaw your personal assistant.\nHere are some commands to configure me:\n\n" +
		"**/about** - Info about Myaaw project\n" +
		"**/me** - About me and show current config\n\n" +
		"**/new** - Start a new chat session\n\n" +
		"ℹ️ You can interact using natural language without needing to set commands first."
}

func CommandAbout() string {
	return "📣 Feel free to contribute to the project!\nhttps://github.com/Shiyinq/myaaw"
}

func CommandNew() string {
	return "✅ A new chat session has been started."
}

func CommandNewFailed() string {
	return "❌ Failed to start a new chat session. Please try again later."
}

func CommandNotFound() string {
	return "4️⃣0️⃣4️⃣ Command not found."
}
