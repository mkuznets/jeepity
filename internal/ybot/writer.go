package ybot

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mkuznets/telebot/v3"
	"golang.org/x/exp/slog"
	"mkuznets.com/go/ytils/ytime"
)

const (
	writerUpdateInterval = 1500 * time.Millisecond
)

type Writer struct {
	buf    *strings.Builder
	bot    *telebot.Bot
	msg    telebot.Editable
	mu     *sync.RWMutex
	cancel context.CancelFunc
}

func NewWriter(ctx context.Context, bot *telebot.Bot, msg telebot.Editable) *Writer {
	cctx, cancel := context.WithCancel(ctx)
	writer := &Writer{
		bot:    bot,
		buf:    &strings.Builder{},
		msg:    msg,
		mu:     &sync.RWMutex{},
		cancel: cancel,
	}
	go writer.doUpdate(cctx)

	return writer
}

func (w *Writer) String() string {
	return w.buf.String()
}

func (w *Writer) doUpdate(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()

	lastMessage := ""

	err := ytime.NewTicker(writerUpdateInterval).
		Start(ctx, func() error {
			message := w.buf.String()
			if strings.TrimSpace(lastMessage) != strings.TrimSpace(message) {
				lastMessage = message
				_, err := w.bot.Edit(w.msg, message)
				if err != nil {
					slog.Error("writer edit", err)
				}
			}

			return nil
		})
	if err != nil {
		slog.Error("writer ticker", err)
	}
}

func (w *Writer) Close() {
	w.cancel()
	w.mu.Lock()
	defer w.mu.Unlock()

	message := w.buf.String()
	_, mErr := w.bot.Edit(w.msg, message, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	if mErr != nil {
		slog.Error("closing writer: markdown edit", mErr)

		_, pErr := w.bot.Edit(w.msg, message)
		if pErr != nil {
			slog.Error("closing writer: plaintext edit", mErr)
		}
	}
}

func (w *Writer) Write(s string) {
	_, _ = w.buf.WriteString(s)
}
