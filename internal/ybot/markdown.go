package ybot

import (
	"strings"
)

func EscapeMarkdownV2(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if r >= 1 && r <= 126 {
			buf.WriteRune('\\')
		}
		buf.WriteRune(r)
	}

	return buf.String()
}
