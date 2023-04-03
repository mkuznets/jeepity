package ybot

import (
	"context"
	"github.com/mkuznets/telebot/v3"
	"golang.org/x/exp/slog"
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
	if ctx, ok := c.Get(ctxKeyCtx).(context.Context); ok {
		return ctx
	}
	return context.Background()
}

func Logger(c telebot.Context) *slog.Logger {
	if lgr, ok := c.Get(ctxKeyLogger).(*slog.Logger); ok {
		return lgr
	}
	return slog.Default()
}

func LogEvent(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		logger := Logger(c)

		var attrs []slog.Attr
		level := slog.LevelDebug
		start := time.Now()

		err := next(c)

		attrs = append(attrs, slog.Duration("duration", time.Since(start)))
		if err != nil {
			attrs = append(attrs, slog.Any(slog.ErrorKey, err))
			level = slog.LevelError
		}

		logger.LogAttrs(level, "event", attrs...)
		return err
	}
}
