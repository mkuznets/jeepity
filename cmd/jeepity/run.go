package main

import (
	"context"
	"fmt"
	"github.com/mkuznets/telebot/v3"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
	"mkuznets.com/go/jeepity/internal/jeepity"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
	"mkuznets.com/go/ytils/yctx"
	"mkuznets.com/go/ytils/yfs"
	"path"
	"runtime/debug"
	"time"
)

type RunCommand struct {
	OpenAiToken        string `long:"openai-token" env:"OPENAI_TOKEN" description:"OpenAI API token" required:"true"`
	TelegramBotToken   string `long:"telegram-bot-token" env:"TELEGRAM_BOT_TOKEN" description:"Telegram bot token" required:"true"`
	DataDir            string `long:"data-dir" env:"DATA_DIR" description:"Data directory" required:"true"`
	EncryptionPassword string `long:"encryption-password" env:"ENCRYPTION_PASSWORD" description:"Data encryption password"`

	ctx     context.Context
	critCtx context.Context
	bot     *telebot.Bot
	st      *store.SqliteStore
	bh      *jeepity.BotHandler
}

func (r *RunCommand) Init(*App) error {
	pref := telebot.Settings{
		Token:  r.TelegramBotToken,
		Poller: &telebot.LongPoller{Timeout: 5 * time.Second},
		OnError: func(err error, c telebot.Context) {
			logger := slog.Default()
			if c != nil {
				logger = ybot.Logger(c)
			}
			logger.Error("unhandled bot error", err, slog.String("err_stack", string(debug.Stack())))
		},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		return err
	}

	ctx, critCtx := yctx.WithTerminator(context.Background())

	st := store.NewSqlite(path.Join(r.DataDir, dbFilename))
	if err := st.Init(ctx); err != nil {
		return fmt.Errorf("sqlite store init: %w", err)
	}

	ai := openai.NewClient(r.OpenAiToken)
	e := jeepity.NewAesEncryptor(r.EncryptionPassword)
	bh := jeepity.NewBotHandler(critCtx, ai, st, e)
	bh.Configure(bot)

	r.ctx = ctx
	r.critCtx = critCtx
	r.bot = bot
	r.st = st
	r.bh = bh

	return nil
}

func (r *RunCommand) Validate() error {
	if _, err := yfs.EnsureDir(r.DataDir); err != nil {
		return err
	}
	return nil
}

func (r *RunCommand) Execute([]string) error {
	g, _ := errgroup.WithContext(r.critCtx)

	g.Go(func() error {
		r.bot.Start()
		return nil
	})

	g.Go(func() error {
		select {
		case <-r.ctx.Done():
			slog.Debug("waiting for handlers to finish")
			r.bh.Wait()

			slog.Debug("stopping bot")
			r.bot.Stop()
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	slog.Debug("cleanup")
	r.st.Close()

	return nil
}
