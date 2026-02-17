package main

import (
	"fmt"
	"strings"
)

// askYesNo prompts the user with a yes/no question.
// defaultYes determines the default answer if the user just presses Enter.
func askYesNo(question string, defaultYes bool) bool {
	prompt := "[y/N]"
	if defaultYes {
		prompt = "[Y/n]"
	}
	fmt.Printf("%s %s: ", question, prompt)

	var input string
	fmt.Scanln(&input)
	input = strings.ToLower(strings.TrimSpace(input))

	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}
