package utils

import (
	"bytes"
	"encoding/json"
	"strings"
)

func FormatErrorMessage(err error) string {
	var formattedJSON bytes.Buffer
	errMessage := err.Error()
	formattedError := errMessage

	if json.Valid([]byte(errMessage)) {
		if err := json.Indent(&formattedJSON, []byte(errMessage), "", "  "); err == nil {
			formattedError = formattedJSON.String()
		}
	} else {
		parts := strings.SplitN(errMessage, ":", 2)
		if len(parts) == 2 && json.Valid([]byte(strings.TrimSpace(parts[1]))) {
			if err := json.Indent(&formattedJSON, []byte(strings.TrimSpace(parts[1])), "", "  "); err == nil {
				formattedError = parts[0] + ":\n" + formattedJSON.String()
			}
		}
	}

	return formattedError
}
