package handlers

import (
	"strings"
	"unicode/utf8"
)

func boundedHandlerText(value string, maxBytes int) string {
	value = strings.ToValidUTF8(value, "")
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
