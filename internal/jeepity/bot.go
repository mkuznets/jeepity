package jeepity

import (
	"context"
	"fmt"
	"github.com/go-pkgz/repeater"
	"github.com/go-pkgz/repeater/strategy"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"gopkg.in/telebot.v3"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
	"time"
)

const (
	ctxKeyUser          = "user"
	initialSystemPrompt = `You are ChatGPT, a large language model trained by OpenAI. Answer as concisely as possible`
	gptModel            = "gpt-3.5-turbo"
	gptUser             = "jeepity"
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
	bot.Use(ybot.AddCtx)
	bot.Use(LogEvent)

	bot.Handle("/start", b.CommandStart, b.ApprovedOnly())
	bot.Handle("/reset", b.CommandReset, b.ApprovedOnly())
	bot.Handle(telebot.OnText, b.OnText, b.ApprovedOnly())
}

func LogEvent(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		attrs := []slog.Attr{
			slog.Int("id", c.Update().ID),
			slog.Int64("chat_id", c.Chat().ID),
			slog.String("username", c.Chat().Username),
		}
		level := slog.LevelDebug
		start := time.Now()

		err := next(c)

		attrs = append(attrs, slog.Duration("duration", time.Since(start)))
		if err != nil {
			attrs = append(attrs, slog.Any(slog.ErrorKey, err))
			level = slog.LevelError
		}

		slog.LogAttrs(level, "EVENT", attrs...)
		return err
	}
}

func (b *BotHandler) ApprovedOnly() telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			ctx := ybot.Ctx(c)

			u, err := b.s.GetUser(ctx, c.Message().Chat.ID)
			if err != nil {
				return err
			}

			if u == nil {
				u = &store.User{
					Approved: true,
					ChatId:   c.Chat().ID,
					Username: c.Chat().Username,
					FullName: c.Chat().FirstName + " " + c.Chat().LastName,
				}
				if err := b.s.PutUser(ctx, u); err != nil {
					return err
				}
			}

			if !u.Approved {
				return c.Send("Hello! Please wait for approval.")
			}

			c.Set(ctxKeyUser, u)

			return next(c)
		}
	}
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

	return c.Send("Chat history has been reset")
}

func (b *BotHandler) OnText(c telebot.Context) error {
	ctx := ybot.Ctx(c)
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
	rErr := repeater.New(backoff).Do(ctx, func() (err error) {
		rctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		resp, err = b.ai.CreateChatCompletion(rctx, req)
		return
	})
	if rErr != nil {
		return c.Send("Could not get a response from OpenAI API. Please try again later.")
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

	return c.Send(chatResponse, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
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
