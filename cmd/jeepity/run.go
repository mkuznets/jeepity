package main

import (
	"github.com/sashabaranov/go-openai"
	"gopkg.in/telebot.v3"
	"mkuznets.com/go/jeepity/internal/jeepity"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/ytils/yfs"
	"path"
	"time"
)

type RunCommand struct {
	OpenAiToken      string `long:"openai-token" env:"OPENAI_TOKEN" description:"OpenAI API token" required:"true"`
	TelegramBotToken string `long:"telegram-bot-token" env:"TELEGRAM_BOT_TOKEN" description:"Telegram bot token" required:"true"`
	DataDir          string `long:"data-dir" env:"DATA_DIR" description:"Data directory" required:"true"`

	bot *telebot.Bot
}

func (r *RunCommand) Init(*App) error {
	pref := telebot.Settings{
		Token:  r.TelegramBotToken,
		Poller: &telebot.LongPoller{Timeout: 5 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		return err
	}

	st := store.NewSqliteStoreFromPath(path.Join(r.DataDir, dbFilename))
	ai := openai.NewClient(r.OpenAiToken)
	bh := jeepity.NewBotHandler(ai, st)
	bh.Configure(bot)

	r.bot = bot

	return nil
}

func (r *RunCommand) Validate() error {
	if _, err := yfs.EnsureDir(r.DataDir); err != nil {
		return err
	}
	return nil
}

func (r *RunCommand) Execute([]string) error {
	r.bot.Start()
	return nil
}
