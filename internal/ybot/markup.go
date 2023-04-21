package ybot

import (
	"github.com/mkuznets/telebot/v3"
)

func SingleButtonMenu(id, text string) *telebot.ReplyMarkup {
	resetMenu := &telebot.ReplyMarkup{}
	resetMenu.ResizeKeyboard = true
	resetButton := resetMenu.Data(text, id)
	resetMenu.Inline(resetMenu.Row(resetButton))
	return resetMenu
}
