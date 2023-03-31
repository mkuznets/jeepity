package jeepity

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-pkgz/repeater"
	"github.com/go-pkgz/repeater/strategy"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"gopkg.in/telebot.v3"
	"gopkg.in/telebot.v3/middleware"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
	"strings"
	"time"
)

const (
	initialSystemPrompt = `You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible`
	gptModel            = "gpt-3.5-turbo"
	gptUser             = "jeepity"
)

var (
	ErrNotApproved    = errors.New("not approved")
	ErrContextTooLong = errors.New("context too long")
	ErrsPersistent    = []error{
		ErrContextTooLong,
	}
)

type BotHandler struct {
	ai *openai.Client
	s  store.Store
}

func NewBotHandler(openAiClient *openai.Client, st store.Store) *BotHandler {
	return &BotHandler{
		ai: openAiClient,
		s:  st,
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
	bot.Use(ybot.AddCtx)

	bot.Use(ybot.LogEvent)
	bot.Use(Authenticate(b.s))

	// # Handlers

	bot.Handle("/start", b.CommandStart)
	bot.Handle("/reset", b.CommandReset)
	bot.Handle(&resetButton, b.CommandReset)
	bot.Handle(telebot.OnText, b.OnText)
}

func (b *BotHandler) CommandStart(c telebot.Context) error {
	return c.Send("Hello! You can start using the bot now")
}

func (b *BotHandler) CommandReset(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	user := c.Get(ctxKeyUser).(*store.User)

	if err := b.s.ClearMessages(ctx, user.ChatId); err != nil {
		return err
	}

	return c.Send("✅ Начат новый диалог. ChatGPT не будет помнить предыдущих сообщений.")
}

func (b *BotHandler) OnText(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	logger := ybot.Logger(c)
	user := c.Get(ctxKeyUser).(*store.User)

	cancel := ybot.NotifyTyping(ctx, c)
	defer cancel()

	var (
		reqMsgs []openai.ChatCompletionMessage
		msgs    []*store.Message
	)

	previousMsgs, err := b.s.GetMessages(ctx, user.ChatId)
	if err != nil {
		return err
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
		Duration: 500 * time.Millisecond,
		Repeats:  5,
		Factor:   1.5,
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
		return fmt.Errorf("no choices returned")
	}

	chatResponse := resp.Choices[0].Message.Content

	msgs = append(msgs, &store.Message{
		ChatId:  user.ChatId,
		Role:    openai.ChatMessageRoleSystem,
		Message: chatResponse,
	})

	if err := b.s.PutMessages(ctx, msgs); err != nil {
		return fmt.Errorf("put messages: %w", err)
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
