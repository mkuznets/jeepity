package locale

import (
	"embed"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"mkuznets.com/go/ytils/y"
)

//go:embed *.toml
var localeFs embed.FS

const (
	defaultLanguage = "en"
)

var Bundle = i18n.NewBundle(language.English)

func init() {
	Bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	_, _ = Bundle.LoadMessageFileFS(localeFs, "locale.en.toml")
	_, _ = Bundle.LoadMessageFileFS(localeFs, "locale.ru.toml")
}

type Locale struct {
	loc *i18n.Localizer
}

func New(lang string) *Locale {
	return &Locale{
		loc: i18n.NewLocalizer(Bundle, lang, defaultLanguage),
	}
}

func (l *Locale) msg(m *i18n.Message) string {
	return y.Must(l.loc.LocalizeMessage(m))
}

func (l *Locale) cfg(cfg *i18n.LocalizeConfig) string {
	return l.loc.MustLocalize(cfg)
}

func (l *Locale) ErrNotApproved() string {
	return l.msg(&i18n.Message{
		ID:    "err_not_approved_message",
		Other: "⛔ You cannot use this bot",
	})
}

func (l *Locale) ErrContextTooLongMessage() string {
	return l.msg(&i18n.Message{
		ID:    "err_context_too_long_message",
		Other: "⛔ The conversation is too long",
	})
}

func (l *Locale) ErrDefaultMessage() string {
	return l.msg(&i18n.Message{
		ID:    "err_default_message",
		Other: "❌ Something went wrong. Please try again",
	})
}

func (l *Locale) ResetBotCommand() string {
	return l.msg(&i18n.Message{
		ID:    "reset_bot_command",
		Other: "Start a new conversation",
	})
}

func (l *Locale) HelpBotCommand() string {
	return l.msg(&i18n.Message{
		ID:    "help_bot_command",
		Other: "Bot description",
	})
}

func (l *Locale) InviteBotCommand() string {
	return l.msg(&i18n.Message{
		ID:    "invite_bot_command",
		Other: "Invite another user to this bot",
	})
}

func (l *Locale) SystemPromptCommand() string {
	return l.msg(&i18n.Message{
		ID:    "system_prompt_command",
		Other: "Update system prompt",
	})
}

func (l *Locale) HelpMessage() string {
	return l.msg(&i18n.Message{
		ID:    "help_message",
		Other: "Hi, I'm Jeepity",
	})
}

func (l *Locale) InviteMessage(url, code string) string {
	return l.cfg(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "invite_message",
			Other: "This bot is invite-only. {{.Url}} {{.Code}}",
		},
		TemplateData: map[string]interface{}{
			"Url":  url,
			"Code": code,
		},
	})
}

func (l *Locale) UnsupportedMessage() string {
	return l.msg(&i18n.Message{
		ID:    "unsupported_message",
		Other: "_Jeepity only supports text messages, audio, and video files_",
	})
}

func (l *Locale) ResetMessage() string {
	return l.msg(&i18n.Message{
		ID:    "reset_message",
		Other: "✅ New conversation initiated. ChatGPT will not remember previous messages.",
	})
}

func (l *Locale) InitialSystemPrompt() string {
	return l.msg(&i18n.Message{
		ID:    "initial_system_prompt",
		Other: "...",
	})
}

func (l *Locale) UpdateSystemPromptMessage(currentPrompt string) string {
	return l.cfg(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "update_system_prompt_message",
			Other: "Update system prompt: {{.CurrentPrompt}}",
		},
		TemplateData: map[string]interface{}{
			"CurrentPrompt": currentPrompt,
		},
	})
}

func (l *Locale) CancelButton() string {
	return l.msg(&i18n.Message{
		ID:    "cancel_button",
		Other: "Cancel",
	})
}

func (l *Locale) DefaultButton() string {
	return l.msg(&i18n.Message{
		ID:    "default_button",
		Other: "Default",
	})
}

func (l *Locale) ResetInlineButton() string {
	return l.msg(&i18n.Message{
		ID:    "reset_inline_button",
		Other: "Start again",
	})
}

func (l *Locale) TranscribeMessage() string {
	return l.msg(&i18n.Message{
		ID:    "transcribe_message",
		Other: "Transcription:",
	})
}

func (l *Locale) SystemPromptUnchanged() string {
	return l.msg(&i18n.Message{
		ID:    "system_prompt_unchanged_message",
		Other: "System prompt not changed",
	})
}

func (l *Locale) SystemPromptUpdatedMessage(newPrompt string) string {
	return l.cfg(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "system_prompt_updated_message",
			Other: "System prompt updated: {{.NewPrompt}}",
		},
		TemplateData: map[string]interface{}{
			"NewPrompt": newPrompt,
		},
	})
}
