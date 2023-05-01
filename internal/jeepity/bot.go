package jeepity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-pkgz/repeater"
	"github.com/go-pkgz/repeater/strategy"
	"github.com/mkuznets/telebot/v3"
	"github.com/mkuznets/telebot/v3/middleware"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"

	"mkuznets.com/go/jeepity/internal/locale"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
)

const (
	initialSystemPrompt = `You are ChatGPT, a large language model trained by OpenAI. Follow the user's instructions carefully. Respond using markdown. Provide very detailed answers with explanations and reasoning.`
	gptModel            = "gpt-3.5-turbo"
	gptUser             = "jeepity"

	backoffDuration = 500 * time.Millisecond
	backoffRepeats  = 5
	backoffFactor   = 1.5
)

var (
	ErrNotApproved    = errors.New("not approved")
	ErrContextTooLong = errors.New("context too long")
	ErrNoChoices      = errors.New("no choices")
	ErrUserNotFound   = errors.New("user not found")
	ErrsPersistent    = []error{
		ErrContextTooLong,
	}
)

type BotHandler struct {
	ctx      context.Context
	bot      *telebot.Bot
	ai       *openai.Client
	s        store.Store
	e        Cryptor
	m        *sync.RWMutex
	stopping *atomic.Bool
}

func NewBotHandler(ctx context.Context, openAiClient *openai.Client, st store.Store, e Cryptor) *BotHandler {
	return &BotHandler{
		ctx:      ctx,
		ai:       openAiClient,
		s:        st,
		e:        e,
		m:        &sync.RWMutex{},
		stopping: &atomic.Bool{},
	}
}

func (b *BotHandler) Configure(bot *telebot.Bot) {
	b.bot = bot

	// # Middleware

	for _, lang := range []string{"en", "ru"} {
		loc := locale.New(lang)
		commands := []telebot.Command{
			{
				Text:        "reset",
				Description: locale.M(loc, &i18n.Message{ID: "reset_bot_command", Other: "Start a new conversation"}),
			},
			{
				Text:        "help",
				Description: locale.M(loc, &i18n.Message{ID: "help_bot_command", Other: "Bot description"}),
			},
		}
		if err := bot.SetCommands(commands, lang); err != nil {
			slog.Error("SetCommands", err, slog.String("lang", lang))
		}
	}

	// ErrorHandler must be the first to catch any possible errors
	// from other middlewares and reply to the user.
	bot.Use(ErrorHandler())

	bot.Use(middleware.Recover())
	bot.Use(ybot.AddLogger)
	bot.Use(middleware.AutoRespond())

	bot.Use(func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			if b.stopping.Load() {
				ybot.Logger(c).Debug("ignoring update because bot is stopping")
				return nil
			}
			return next(c)
		}
	})
	bot.Use(ybot.TakeMutex(b.m))
	bot.Use(ybot.Sequential(func(c telebot.Context) string {
		return fmt.Sprintf("%d", c.Sender().ID)
	}))

	bot.Use(ybot.AddCtx(b.ctx))

	bot.Use(ybot.LogEvent)
	bot.Use(Authenticate(b.s))

	// # Handlers

	bot.Handle("/start", b.CommandHelp, ybot.AddTag("start"))
	bot.Handle("/help", b.CommandHelp, ybot.AddTag("help"))
	bot.Handle("/reset", b.CommandReset, ybot.AddTag("reset"))
	bot.Handle(&telebot.Btn{Unique: "reset"}, b.CommandReset, ybot.AddTag("reset_button"))
	bot.Handle(telebot.OnVoice, b.Transcribe, ybot.AddTag("transcribe"))
	bot.Handle(telebot.OnText, b.Complete, ybot.AddTag("chat_completion"))
	bot.Handle(telebot.OnMedia, b.Unsupported, ybot.AddTag("media"))
}

func (b *BotHandler) Wait() {
	b.stopping.Store(true)
	defer b.m.Unlock()
	b.m.Lock()
}

func (b *BotHandler) CommandHelp(c telebot.Context) error {
	loc := locale.New(ybot.Lang(c))
	msg := locale.M(loc, &i18n.Message{ID: "help_message", Other: "Hi, I'm Jeepity"})
	return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown, DisableWebPagePreview: true})
}

func (b *BotHandler) Unsupported(c telebot.Context) error {
	loc := locale.New(ybot.Lang(c))
	msg := locale.M(loc, &i18n.Message{ID: "unsupported_message", Other: "_Jeepity only supports text messages_"})
	return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdownV2})
}

func (b *BotHandler) CommandReset(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}
	loc := locale.New(ybot.Lang(c))

	if err := b.s.ClearMessages(ctx, user.ChatId); err != nil {
		return err
	}

	msg := locale.M(loc, &i18n.Message{
		ID:    "reset_message",
		Other: "✅ New conversation initiated. ChatGPT will not remember previous messages.",
	})
	return c.Send(msg)
}

func (b *BotHandler) Transcribe(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	logger := ybot.Logger(c)

	isForwarded := c.Message().OriginalUnixtime != 0

	cancelNotify := ybot.NotifyTyping(ctx, c)
	defer cancelNotify()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	tmpFile, err := os.CreateTemp("", "jeepity-voice*.ogg")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	mp3FilePath := tmpFile.Name() + ".mp3"

	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		_ = os.Remove(mp3FilePath)
	}()

	if err := b.bot.Download(&c.Message().Voice.File, tmpFile.Name()); err != nil {
		return fmt.Errorf("download voice message: %w", err)
	}
	_ = tmpFile.Sync()

	logger.Debug("voice file downloaded", slog.String("path", tmpFile.Name()))

	conv := NewOggMp3Converter(tmpFile.Name(), mp3FilePath)
	if err := conv.Command(ctx).Run(); err != nil {
		return fmt.Errorf("convert voice message: %w", err)
	}

	resp, err := b.ai.CreateTranscription(ctx, openai.AudioRequest{
		Model:    openai.Whisper1,
		FilePath: mp3FilePath,
	})
	if err != nil {
		return fmt.Errorf("CreateTranscription: %w", err)
	}

	loc := locale.New(ybot.Lang(c))
	systemMsg := locale.M(loc, &i18n.Message{ID: "transcribe_message", Other: "Transcription:"})

	err = c.Send(systemMsg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	err = c.Send(resp.Text)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	if isForwarded {
		return nil
	}

	return b.doCompletion(ctx, c, resp.Text)
}

func (b *BotHandler) Complete(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	cancel := ybot.NotifyTyping(ctx, c)
	defer cancel()

	return b.doCompletion(ctx, c, c.Message().Text)
}

func (b *BotHandler) doCompletion(ctx context.Context, c telebot.Context, text string) error {
	logger := ybot.Logger(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}

	var (
		reqMsgs []openai.ChatCompletionMessage
		msgs    []*store.Message
	)

	previousMsgs, err := b.s.GetDialogMessages(ctx, user.ChatId)
	if err != nil {
		return err
	}

	for _, msg := range previousMsgs {
		if err := b.e.DecryptMessage(user, msg); err != nil {
			return fmt.Errorf("message id=%d DecryptMessage: %w", msg.Id, err)
		}
	}

	if len(previousMsgs) > 0 {
		reqMsgs = messagesToOpenAiMessages(previousMsgs)
	} else {
		msgs = append(msgs, &store.Message{
			ChatId:  user.ChatId,
			Role:    openai.ChatMessageRoleSystem,
			Message: initialSystemPrompt,
		})
	}

	msgs = append(msgs, &store.Message{
		ChatId:  user.ChatId,
		Role:    openai.ChatMessageRoleUser,
		Message: text,
	})

	reqMsgs = append(reqMsgs, messagesToOpenAiMessages(msgs)...)

	req := openai.ChatCompletionRequest{
		Model:    gptModel,
		User:     gptUser,
		Messages: reqMsgs,
	}

	var resp openai.ChatCompletionResponse

	backoff := &strategy.Backoff{
		Duration: backoffDuration,
		Repeats:  backoffRepeats,
		Factor:   backoffFactor,
		Jitter:   true,
	}

	makeCompletion := func() error {
		attrs := []slog.Attr{
			slog.Int("context_length", len(reqMsgs)),
		}
		level := slog.LevelDebug
		defer func() {
			logger.LogAttrs(level, "CreateChatCompletion", attrs...)
		}()

		start := time.Now()

		rctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		resp, err = b.ai.CreateChatCompletion(rctx, req)

		attrs = append(attrs,
			slog.Duration("duration", time.Since(start)),
			slog.Int("prompt_tokens", resp.Usage.PromptTokens),
			slog.Int("completion_tokens", resp.Usage.CompletionTokens),
			slog.Int("total_tokens", resp.Usage.TotalTokens),
		)

		if err != nil {
			attrs = append(attrs, slog.Any(slog.ErrorKey, err))
			level = slog.LevelError
			if strings.Contains(err.Error(), "reduce the length of the messages") {
				return ErrContextTooLong
			}
			return err
		}
		return nil
	}

	if err := repeater.New(backoff).Do(ctx, makeCompletion, ErrsPersistent...); err != nil {
		return err
	}

	if len(resp.Choices) < 1 {
		return ErrNoChoices
	}

	chatResponse := resp.Choices[0].Message.Content

	msgs = append(msgs, &store.Message{
		ChatId:  user.ChatId,
		Role:    openai.ChatMessageRoleSystem,
		Message: chatResponse,
	})

	for _, msg := range msgs {
		if err := b.e.EncryptMessage(user, msg); err != nil {
			return fmt.Errorf("message encrypt: %w", err)
		}
	}

	if err := b.s.PutMessages(ctx, msgs); err != nil {
		return fmt.Errorf("put messages: %w", err)
	}

	usage := &store.Usage{
		ChatId:           c.Sender().ID,
		UpdateId:         c.Update().ID,
		Model:            resp.Model,
		CompletionTokens: resp.Usage.CompletionTokens,
		PromptTokens:     resp.Usage.PromptTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}

	if err := b.s.PutUsage(ctx, usage); err != nil {
		logger.Error("PutUsage", err)
	}

	mErr := c.Send(chatResponse, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	if mErr != nil {
		logger.Error("send markdown", mErr)
		return c.Send(chatResponse, &telebot.SendOptions{ParseMode: telebot.ModeDefault})
	}

	return nil
}

func messagesToOpenAiMessages(messages []*store.Message) []openai.ChatCompletionMessage {
	res := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		res[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Message,
		}
	}
	return res
}
