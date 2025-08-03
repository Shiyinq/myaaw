package utils

func Watermark(content string, model string, active bool) string {
	if active {
		return content + "\n\n🤖 *" + model + "*"
	}
	return content
}
