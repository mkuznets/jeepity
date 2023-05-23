package main

import (
	"context"
	"fmt"
	"path"
	"runtime/debug"
	"time"

	"github.com/mkuznets/telebot/v3"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
	"mkuznets.com/go/ytils/yctx"
	"mkuznets.com/go/ytils/yfs"

	"mkuznets.com/go/jeepity/internal/jeepity"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
)

const (
	longPollTimeout       = 10 * time.Second
	maxWebhookConnections = 16
)

type RunCommand struct {
	OpenAi   *OpenAi   `group:"OpenAI parameters" namespace:"openai" env-namespace:"OPENAI"`
	Telegram *Telegram `group:"Telegram parameters" namespace:"telegram" env-namespace:"TELEGRAM"`
	Data     *Data     `group:"Data parameters" namespace:"data" env-namespace:"DATA"`
}

type OpenAi struct {
	Token      string `long:"token" env:"TOKEN" description:"OpenAI API token" required:"true"`
	ChatModel  string `long:"chat-model" env:"CHAT_MODEL" description:"OpenAI chat model" default:"gpt-3.5-turbo-0301"`
	AudioModel string `long:"audio-model" env:"AUDIO_MODEL" description:"OpenAI audio transctiption model" default:"whisper-1"`
}

type Telegram struct {
	BotToken string   `long:"bot-token" env:"BOT_TOKEN" description:"Telegram bot token" required:"true"`
	Mode     string   `long:"mode" env:"MODE" description:"Method to receive updates" default:"polling" choice:"polling" choice:"webhook"`
	Webhook  *Webhook `group:"Telegram webhook parameters (only apply for MODE=webhook)" namespace:"webhook" env-namespace:"WEBHOOK"`
}

type Webhook struct {
	Addr   string `long:"addr" env:"ADDR" description:"Webhook listen address" default:":8080"`
	Url    string `long:"url" env:"URL" description:"Webhook URL"`
	Secret string `long:"secret" env:"SECRET" description:"Webhook secret"`
}

type Data struct {
	Dir                string `long:"dir" env:"DIR" description:"Database directory" required:"true"`
	EncryptionPassword string `long:"encryption-password" env:"ENCRYPTION_PASSWORD" description:"Encryption password for messages"`
}

func (r *RunCommand) Init(*App) error {
	return nil
}

func (r *RunCommand) Validate() error {
	if _, err := yfs.EnsureDir(r.Data.Dir); err != nil {
		return fmt.Errorf("EnsureDir: %w", err)
	}

	if r.Telegram.Mode == "webhook" {
		if r.Telegram.Webhook.Url == "" {
			return fmt.Errorf("TELEGRAM_WEBHOOK_URL is required")
		}
	}

	return nil
}

func (r *RunCommand) Execute([]string) error {
	var poller telebot.Poller

	switch r.Telegram.Mode {
	case "polling":
		poller = &telebot.LongPoller{Timeout: longPollTimeout}
	case "webhook":
		poller = &telebot.Webhook{
			Listen:         r.Telegram.Webhook.Addr,
			MaxConnections: maxWebhookConnections,
			SecretToken:    r.Telegram.Webhook.Secret,
			Endpoint: &telebot.WebhookEndpoint{
				PublicURL: r.Telegram.Webhook.Url,
			},
		}
	}

	pref := telebot.Settings{
		Token:  r.Telegram.BotToken,
		Poller: poller,
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
		return fmt.Errorf("NewBot: %w", err)
	}

	ctx, critCtx := yctx.WithTerminator(context.Background())

	st, err := store.NewSqlite(path.Join(r.Data.Dir, dbFilename))
	if err != nil {
		return fmt.Errorf("store.NewSqlite: %w", err)
	}

	inviteCode := ybot.InviteCode()
	st.SetDefaultInviteCode(inviteCode)

	ai := openai.NewClient(r.OpenAi.Token)
	e := jeepity.NewAesEncryptor(r.Data.EncryptionPassword)
	bh := jeepity.NewBotHandler(critCtx, ai, st, e)
	bh.Configure(bot)

	g, _ := errgroup.WithContext(critCtx)

	g.Go(func() error {
		slog.Debug("Starting Telegram bot...")
		slog.Info(fmt.Sprintf("Invite URL: %s", ybot.InviteUrl(bot.Me.Username, inviteCode)))
		slog.Info("(This URL is temporary, DO NOT SHARE IT WITH ANYONE)")

		bot.Start()
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()

		slog.Debug("waiting for handlers to finish")
		bh.Wait()

		slog.Debug("stopping bot")
		bot.Stop()

		return nil
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("errgroup: %w", err)
	}

	slog.Debug("store cleanup")
	st.Close()

	slog.Debug("call /deleteWebhook")
	if err := bot.RemoveWebhook(); err != nil {
		slog.Error("RemoveWebhook", err)
	}

	return nil
}
