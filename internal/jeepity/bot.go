package jeepity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-pkgz/repeater"
	"github.com/go-pkgz/repeater/strategy"
	"github.com/mkuznets/telebot/v3"
	"github.com/mkuznets/telebot/v3/middleware"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"

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
	// # Menus and buttons

	resetMenu := bot.NewMarkup()
	resetMenu.ResizeKeyboard = true
	resetButton := resetMenu.Data("Начать заново", "reset")
	resetMenu.Inline(resetMenu.Row(resetButton))

	// # Middleware

	// ErrorHandler must be the first to catch any possible errors
	// from other middlewares and reply to the user.
	bot.Use(ErrorHandler(resetMenu))

	bot.Use(middleware.Recover())
	bot.Use(ybot.AddLogger)

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

	bot.Use(ybot.AddCtx(b.ctx))

	bot.Use(ybot.LogEvent)
	bot.Use(Authenticate(b.s))

	// # Handlers

	bot.Handle("/start", b.CommandStart, ybot.AddTag("start"))
	bot.Handle("/reset", b.CommandReset, ybot.AddTag("reset"))
	bot.Handle(&resetButton, b.CommandReset, ybot.AddTag("reset_button"))
	bot.Handle(telebot.OnText, b.OnText, ybot.AddTag("chat_completion"))
}

func (b *BotHandler) Wait() {
	b.stopping.Store(true)
	defer b.m.Unlock()
	b.m.Lock()
}

func (b *BotHandler) CommandStart(c telebot.Context) error {
	return c.Send("Hello! You can start using the bot now")
}

func (b *BotHandler) CommandReset(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}

	if err := b.s.ClearMessages(ctx, user.ChatId); err != nil {
		return err
	}

	return c.Send("✅ Начат новый диалог. ChatGPT не будет помнить предыдущих сообщений.")
}

func (b *BotHandler) OnText(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	logger := ybot.Logger(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}

	cancel := ybot.NotifyTyping(ctx, c)
	defer cancel()

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
		Message: c.Text(),
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
