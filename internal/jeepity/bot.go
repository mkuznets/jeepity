package jeepity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-pkgz/repeater"
	"github.com/go-pkgz/repeater/strategy"
	"github.com/h2non/filetype"
	"github.com/mkuznets/telebot/v3"
	"github.com/mkuznets/telebot/v3/middleware"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
	"mkuznets.com/go/ytils/ylog"
	"ytils.dev/heartbeat"

	"mkuznets.com/go/jeepity/internal/locale"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
)

const (
	gptUser         = "jeepity"
	defaultGptModel = openai.GPT3Dot5Turbo

	backoffDuration = 500 * time.Millisecond
	backoffRepeats  = 5
	backoffFactor   = 1.5

	completionTotalTimeout  = 5 * time.Minute
	streamIdleTimeout       = 30 * time.Second
	streamIdleCheckInterval = 5 * time.Second
)

var (
	ErrNotApproved    = errors.New("not approved")
	ErrContextTooLong = errors.New("context too long")
	ErrUserNotFound   = errors.New("user not found")
	ErrsPersistent    = []error{
		ErrContextTooLong,
	}
)

type Completion struct {
	Model            string
	Response         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

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
				Description: loc.ResetBotCommand(),
			},
			{
				Text:        "help",
				Description: loc.HelpBotCommand(),
			},
			{
				Text:        "invite",
				Description: loc.InviteBotCommand(),
			},
			{
				Text:        "prompt",
				Description: loc.SystemPromptCommand(),
			},
		}
		if err := bot.SetCommands(commands, lang); err != nil {
			slog.Error("SetCommands", ylog.Err(err), slog.String("lang", lang))
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

	bot.Handle(&telebot.Btn{Unique: "reset_chat_context"}, b.CommandReset, ybot.AddTag("reset_button"))
	bot.Handle(&telebot.Btn{Unique: "cancel_state"}, b.ClearInputState, ybot.AddTag("cancel_state_button"))
	bot.Handle(&telebot.Btn{Unique: "set_default_system_prompt"}, b.SetDefaultSystemPrompt, ybot.AddTag("set_default_system_prompt_button"))

	bot.Handle("/start", b.CommandHelp, ybot.AddTag("start"))
	bot.Handle("/help", b.CommandHelp, ybot.AddTag("help"))
	bot.Handle("/invite", b.CommandInvite, ybot.AddTag("invite"))
	bot.Handle("/reset", b.CommandReset, ybot.AddTag("reset"))
	bot.Handle("/prompt", b.CommandSystemPrompt, ybot.AddTag("system_prompt"))

	bot.Handle(telebot.OnText, b.Text, ybot.AddTag("chat_completion"))
	bot.Handle(telebot.OnVoice, b.TranscribeVoice, ybot.AddTag("transcribe_voice"))
	bot.Handle(telebot.OnAudio, b.TranscribeAudio, ybot.AddTag("transcribe_audio"))
	bot.Handle(telebot.OnVideo, b.TranscribeVideo, ybot.AddTag("transcribe_video"))
	bot.Handle(telebot.OnVideoNote, b.TranscribeVideoNote, ybot.AddTag("transcribe_video_note"))
	bot.Handle(telebot.OnDocument, b.TranscribeDocument, ybot.AddTag("transcribe_document"))

	bot.Handle(telebot.OnMedia, b.Unsupported, ybot.AddTag("media"))
}

func (b *BotHandler) Wait() {
	b.stopping.Store(true)
	defer b.m.Unlock()
	b.m.Lock()
}

func (b *BotHandler) CommandHelp(c telebot.Context) error {
	loc := locale.New(ybot.Lang(c))
	return c.Send(loc.HelpMessage(), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown, DisableWebPagePreview: true})
}

func (b *BotHandler) CommandInvite(c telebot.Context) error {
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}

	loc := locale.New(ybot.Lang(c))

	msg := loc.InviteMessage(
		ybot.InviteUrl(b.bot.Me.Username, user.InviteCode),
		user.InviteCode,
	)

	return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeHTML, DisableWebPagePreview: true})
}

func (b *BotHandler) Unsupported(c telebot.Context) error {
	loc := locale.New(ybot.Lang(c))
	return c.Send(loc.UnsupportedMessage(), &telebot.SendOptions{ParseMode: telebot.ModeMarkdownV2})
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
	if err := b.s.ResetDiglogID(ctx, user); err != nil {
		return err
	}

	loc := locale.New(ybot.Lang(c))
	return c.Send(loc.ResetMessage())
}

func (b *BotHandler) ClearInputState(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}

	if err := b.s.SetInputState(ctx, user.ChatId, store.InputStateEmpty); err != nil {
		return err
	}

	return c.Send("OK!")
}

func (b *BotHandler) SetDefaultSystemPrompt(c telebot.Context) error {
	return b.doSetSystemPrompt(c, "")
}

func (b *BotHandler) CommandSystemPrompt(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}
	loc := locale.New(ybot.Lang(c))

	currentPrompt := user.SystemPrompt
	if currentPrompt == "" {
		currentPrompt = loc.InitialSystemPrompt()
	}

	if err := b.s.SetInputState(ctx, user.ChatId, store.InputStateWaitingForSystemPrompt); err != nil {
		return fmt.Errorf("set state: %w", err)
	}

	msg := loc.UpdateSystemPromptMessage(ybot.EscapeMarkdownV2(currentPrompt))

	menuItems := []string{
		"cancel_state", loc.CancelButton(),
	}
	if user.SystemPrompt != "" {
		menuItems = append([]string{"set_default_system_prompt", loc.DefaultButton()}, menuItems...)
	}
	menu := ybot.MultiButtonMenu(menuItems...)

	return c.Send(msg, &telebot.SendOptions{
		ParseMode:   telebot.ModeMarkdownV2,
		ReplyMarkup: menu,
	})
}

func (b *BotHandler) TranscribeDocument(c telebot.Context) error {
	return b.transcribe(c, &c.Message().Document.File, false)
}

func (b *BotHandler) TranscribeVoice(c telebot.Context) error {
	return b.transcribe(c, &c.Message().Voice.File, true)
}

func (b *BotHandler) TranscribeAudio(c telebot.Context) error {
	return b.transcribe(c, &c.Message().Audio.File, false)
}

func (b *BotHandler) TranscribeVideo(c telebot.Context) error {
	return b.transcribe(c, &c.Message().Video.File, false)
}

func (b *BotHandler) TranscribeVideoNote(c telebot.Context) error {
	return b.transcribe(c, &c.Message().VideoNote.File, false)
}

func (b *BotHandler) transcribe(c telebot.Context, file *telebot.File, completion bool) error {
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

	if err := b.bot.Download(file, tmpFile.Name()); err != nil {
		return fmt.Errorf("download voice message: %w", err)
	}
	_ = tmpFile.Sync()

	fileType, err := filetype.MatchFile(tmpFile.Name())
	if err != nil {
		return b.Unsupported(c)
	}
	if fileType.MIME.Type != "audio" && fileType.MIME.Type != "video" {
		return b.Unsupported(c)
	}

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

	err = c.Send(loc.TranscribeMessage(), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	err = c.Send(resp.Text)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	if isForwarded || !completion {
		return nil
	}

	return b.doCompletion(ctx, c, resp.Text)
}

func (b *BotHandler) Text(c telebot.Context) error {
	ctx := ybot.Ctx(c)
	cancel := ybot.NotifyTyping(ctx, c)
	defer cancel()

	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}

	switch user.InputState {
	case store.InputStateEmpty:
		return b.doCompletion(ctx, c, c.Message().Text)

	case store.InputStateWaitingForSystemPrompt:
		return b.doSetSystemPrompt(c, c.Message().Text)
	}

	return nil
}

func (b *BotHandler) doSetSystemPrompt(c telebot.Context, prompt string) error {
	ctx := ybot.Ctx(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}
	loc := locale.New(ybot.Lang(c))

	if user.SystemPrompt == prompt {
		return c.Send(loc.SystemPromptUnchanged())
	}

	if err := b.s.SetSystemPrompt(ctx, user.ChatId, prompt); err != nil {
		return fmt.Errorf("SetSystemPrompt: %w", err)
	}

	if err := b.s.SetInputState(ctx, user.ChatId, store.InputStateEmpty); err != nil {
		return fmt.Errorf("SetInputState: %w", err)
	}

	if err := b.s.ClearMessages(ctx, user.ChatId); err != nil {
		return fmt.Errorf("ClearMessages: %w", err)
	}

	displayPrompt := prompt
	if displayPrompt == "" {
		displayPrompt = loc.InitialSystemPrompt()
	}

	msg := loc.SystemPromptUpdatedMessage(ybot.EscapeMarkdownV2(displayPrompt))

	return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdownV2})
}

func (b *BotHandler) doCompletion(ctx context.Context, c telebot.Context, text string) error {
	logger := ybot.Logger(c)
	user, ok := c.Get(ctxKeyUser).(*store.User)
	if !ok {
		return ErrUserNotFound
	}
	loc := locale.New(ybot.Lang(c))

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
		systemPrompt := user.SystemPrompt
		if systemPrompt == "" {
			systemPrompt = loc.InitialSystemPrompt()
		}

		msgs = append(msgs, &store.Message{
			ChatId:  user.ChatId,
			Role:    openai.ChatMessageRoleSystem,
			Message: systemPrompt,
		})
	}

	msgs = append(msgs, &store.Message{
		ChatId:  user.ChatId,
		Role:    openai.ChatMessageRoleUser,
		Message: text,
	})

	reqMsgs = append(reqMsgs, messagesToOpenAiMessages(msgs)...)

	model := user.Model
	if model == "" {
		model = defaultGptModel
	}

	req := openai.ChatCompletionRequest{
		Model:    model,
		User:     gptUser,
		Messages: reqMsgs,
	}

	backoff := &strategy.Backoff{
		Duration: backoffDuration,
		Repeats:  backoffRepeats,
		Factor:   backoffFactor,
		Jitter:   true,
	}

	reply, err := b.bot.Send(c.Recipient(), "...")
	if err != nil {
		return err
	}

	var completion *Completion
	completeFunc := func() error {
		attrs := []slog.Attr{
			slog.Int("context_length", len(reqMsgs)),
		}
		level := slog.LevelDebug
		defer func() {
			logger.LogAttrs(ctx, level, "completion", attrs...)
		}()

		start := time.Now()

		r, cErr := b.makeStreamCompletion(ctx, reply, &req)

		attrs = append(attrs, slog.Duration("duration", time.Since(start)))

		if cErr != nil {
			attrs = append(attrs, ylog.Err(cErr))
			level = slog.LevelError
			if strings.Contains(cErr.Error(), "reduce the length of the messages") {
				return ErrContextTooLong
			}
			return cErr
		}

		attrs = append(attrs,
			slog.String("model", r.Model),
			slog.Int("prompt_tokens", r.PromptTokens),
			slog.Int("completion_tokens", r.CompletionTokens),
			slog.Int("total_tokens", r.TotalTokens),
		)
		completion = r

		return nil
	}

	if err := repeater.New(backoff).Do(ctx, completeFunc, ErrsPersistent...); err != nil {
		return err
	}

	msgs = append(msgs, &store.Message{
		ChatId:  user.ChatId,
		Role:    openai.ChatMessageRoleAssistant,
		Message: completion.Response,
	})

	for _, msg := range msgs {
		if err := b.e.EncryptMessage(user, msg); err != nil {
			return fmt.Errorf("message encrypt: %w", err)
		}
	}

	if err := b.s.PutMessages(ctx, msgs); err != nil {
		return fmt.Errorf("put messages: %w", err)
	}

	return nil
}

func (b *BotHandler) makeStreamCompletion(ctx context.Context, responseMsg telebot.Editable, req *openai.ChatCompletionRequest) (*Completion, error) {
	ctx, cancel := context.WithTimeout(ctx, completionTotalTimeout)
	defer cancel()

	hb := heartbeat.New(ctx, streamIdleTimeout, &heartbeat.Options{
		CheckInterval: streamIdleCheckInterval,
	})
	defer hb.Close()

	writer := ybot.NewWriter(hb.Ctx(), b.bot, responseMsg)

	completion := &Completion{}

	g, ctx := errgroup.WithContext(hb.Ctx())
	g.Go(func() error {
		stream, err := b.ai.CreateChatCompletionStream(ctx, *req)
		if err != nil {
			return fmt.Errorf("CreateChatCompletion: %w", err)
		}
		defer writer.Close()
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return fmt.Errorf("stream.Recv: %w", err)
			}

			completion.Model = response.Model
			if len(response.Choices) > 0 {
				writer.Write(response.Choices[0].Delta.Content)
				hb.Beat()
			}
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	completion.Response = writer.String()

	return completion, nil
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
