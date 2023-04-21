package jeepity

import (
	"errors"
	"fmt"

	"github.com/mkuznets/telebot/v3"
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"mkuznets.com/go/jeepity/internal/locale"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
)

const ctxKeyUser = "user"

func Authenticate(s store.Store) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			ctx := ybot.Ctx(c)
			sender := c.Sender()

			u, err := s.GetUser(ctx, sender.ID)
			if err != nil {
				return fmt.Errorf("GetUser: %w", err)
			}

			if u == nil {
				u = &store.User{
					Approved: true,
					ChatId:   sender.ID,
					Username: sender.Username,
					FullName: sender.FirstName + " " + sender.LastName,
				}
				newUser, err := s.PutUser(ctx, u)
				if err != nil {
					return fmt.Errorf("PutUser: %w", err)
				}
				u = newUser
			}

			if !u.Approved {
				return ErrNotApproved
			}

			c.Set(ctxKeyUser, u)

			return next(c)
		}
	}
}

func ErrorHandler() telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			err := next(c)
			if err == nil {
				return nil
			}

			loc := locale.New(ybot.Lang(c))

			switch {
			case errors.Is(err, ErrNotApproved):
				msg := locale.M(loc, &i18n.Message{ID: "err_not_approved_message", Other: "⛔ You cannot use this bot"})
				return c.Send(msg)
			case errors.Is(err, ErrContextTooLong):
				resetButtonText := locale.M(loc, &i18n.Message{ID: "reset_inline_button", Other: "Start again"})
				msg := locale.M(loc, &i18n.Message{ID: "err_context_too_long_message", Other: "⛔ The conversation is too long"})
				return c.Send(msg, ybot.SingleButtonMenu("reset", resetButtonText))
			default:
				msg := locale.M(loc, &i18n.Message{ID: "err_default_message", Other: "❌ Something went wrong. Please try again"})
				return c.Send(msg)
			}
		}
	}
}
