package ybot

import "fmt"

func InviteUrl(username, code string) string {
	return fmt.Sprintf("https://t.me/%s?start=%s", username, code)
}
