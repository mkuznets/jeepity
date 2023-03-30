package ybot

import (
	"context"
	"golang.org/x/exp/slog"
	"gopkg.in/telebot.v3"
	"mkuznets.com/go/ytils/ytime"
	"time"
)

const (
	ctxKeyCtx    = "ctx"
	ctxKeyLogger = "lgr"
)

func AddCtx(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		ctx := context.Background()
		c.Set(ctxKeyCtx, ctx)
		return next(c)
	}
}

func AddLogger(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		logger := slog.With(
			slog.Int("id", c.Update().ID),
			slog.Int64("chat_id", c.Chat().ID),
			slog.String("username", c.Message().Sender.Username),
		)
		c.Set(ctxKeyLogger, logger)
		return next(c)
	}
}

func Ctx(c telebot.Context) context.Context {
	return c.Get(ctxKeyCtx).(context.Context)
}

func Logger(c telebot.Context) *slog.Logger {
	return c.Get(ctxKeyLogger).(*slog.Logger)
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
