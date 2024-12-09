package utils

func Watermark(content string, model string) string {
	return content + "\n\n🤖 *" + model + "*"
}
