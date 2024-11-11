package utils

import (
	"time"
)

func GetCurrentTime() string {
	return time.Now().Format("Monday, 02-Jan-2006 15:04:05")
}
