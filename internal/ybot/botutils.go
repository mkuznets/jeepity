package ybot

import (
	"context"
	"gopkg.in/telebot.v3"
	"mkuznets.com/go/ytils/ytime"
	"time"
)

const (
	ctxKeyCtx = "ctx"
)

func AddCtx(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		ctx := context.Background()
		c.Set(ctxKeyCtx, ctx)
		return next(c)
	}
}

func Ctx(c telebot.Context) context.Context {
	return c.Get(ctxKeyCtx).(context.Context)
}

func NotifyTyping(ctx context.Context, c telebot.Context) context.CancelFunc {
	tctx, cancel := context.WithCancel(ctx)
	go func() {
		_ = ytime.NewTicker(5*time.Second).Start(tctx, func() error {
			return c.Notify(telebot.Typing)
		})
	}()

	return cancel
}
