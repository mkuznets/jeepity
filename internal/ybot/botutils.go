package ybot

import (
	"context"
	"github.com/mkuznets/telebot/v3"
	"mkuznets.com/go/ytils/ytime"
	"time"
)

func NotifyTyping(ctx context.Context, c telebot.Context) context.CancelFunc {
	tctx, cancel := context.WithCancel(ctx)
	go func() {
		_ = ytime.NewTicker(5*time.Second).Start(tctx, func() error {
			return c.Notify(telebot.Typing)
		})
	}()

	return cancel
}
