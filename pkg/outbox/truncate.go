package outbox

import (
	"unicode/utf8"
)

func truncateError(err error, maxBytes int) string {
	if err == nil {
		return ""
	}
	return truncateString(err.Error(), maxBytes)
}

func truncateString(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	b := []byte(s[:maxBytes])
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	if len(b) == 0 {
		return ""
	}
	return string(b)
}
