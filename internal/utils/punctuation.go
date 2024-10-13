package utils

import "strings"

func ContainsPunctuation(text string) bool {
	conditions := []string{".", "\n", "!", "?"}
	for _, c := range conditions {
		if strings.Contains(text, c) {
			return true
		}
	}
	return false
}
