package utils

func GetImageCaption(caption string) string {
	if caption != "" {
		return caption
	}
	return "Explain this image"
}
