package ybot

import (
	"github.com/mkuznets/telebot/v3"
)

func SingleButtonMenu(id, text string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	menu.ResizeKeyboard = true
	button := menu.Data(text, id)
	menu.Inline(menu.Row(button))
	return menu
}

func MultiButtonMenu(arg ...string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	menu.ResizeKeyboard = true

	var buttons []telebot.Btn
	for i := 0; i < len(arg); i += 2 {
		buttons = append(buttons, menu.Data(arg[i+1], arg[i]))
	}
	menu.Inline(menu.Row(buttons...))
	return menu
}
