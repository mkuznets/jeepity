package ybot

import (
	"context"
	"fmt"
	"time"

	"github.com/mkuznets/telebot/v3"
	"mkuznets.com/go/ytils/ytime"
)

const notifyTypingInterval = 5 * time.Second

func NotifyTyping(ctx context.Context, c telebot.Context) context.CancelFunc {
	tctx, cancel := context.WithCancel(ctx)
	go func() {
		_ = ytime.NewTicker(notifyTypingInterval).Start(tctx, func() error {
			if err := c.Notify(telebot.Typing); err != nil {
				return fmt.Errorf("notify typing: %w", err)
			}
			return nil
		})
	}()

	return cancel
}
