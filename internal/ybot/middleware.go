package ybot

import (
	"context"
	"sync"
	"time"

	"github.com/mkuznets/telebot/v3"
	"golang.org/x/exp/slog"
)

const (
	ctxKeyCtx    = "ctx"
	ctxKeyLogger = "lgr"
	ctxKeyTag    = "tag"
)

func AddCtx(ctx context.Context) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			c.Set(ctxKeyCtx, ctx)
			return next(c)
		}
	}
}

func TakeMutex(m *sync.RWMutex) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			m.RLock()
			defer m.RUnlock()
			return next(c)
		}
	}
}

func AddLogger(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		logger := slog.With(
			slog.Int("id", c.Update().ID),
			slog.Int64("chat_id", c.Sender().ID),
			slog.String("username", c.Sender().Username),
		)
		c.Set(ctxKeyLogger, logger)
		return next(c)
	}
}

func Lang(c telebot.Context) string {
	lang := c.Sender().LanguageCode
	if lang == "" {
		lang = "en"
	}
	return lang
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

func Tag(c telebot.Context) string {
	if tag, ok := c.Get(ctxKeyTag).(string); ok {
		return tag
	}
	return ""
}

func AddTag(tag string) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			c.Set(ctxKeyTag, tag)
			if lgr, ok := c.Get(ctxKeyLogger).(*slog.Logger); ok && lgr != nil {
				lgr = lgr.With(slog.String("tag", tag))
				c.Set(ctxKeyLogger, lgr)
			}
			return next(c)
		}
	}
}

func LogEvent(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		var attrs []slog.Attr
		level := slog.LevelDebug
		start := time.Now()

		err := next(c)

		logger := Logger(c)

		attrs = append(attrs, slog.Duration("duration", time.Since(start)))
		if err != nil {
			attrs = append(attrs, slog.Any(slog.ErrorKey, err))
			level = slog.LevelError
		}

		logger.LogAttrs(level, "event", attrs...)
		return err
	}
}
