package jeepity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mkuznets/telebot/v3"

	"mkuznets.com/go/jeepity/internal/locale"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
)

const ctxKeyUser = "user"

func extractInviteCode(c telebot.Context) string {
	msg := c.Message()
	if msg == nil {
		return ""
	}

	if !strings.HasPrefix(msg.Text, "/start") {
		return ""
	}

	args := c.Args()
	if len(args) != 1 {
		return ""
	}

	code := args[0]
	if len(code) < 1 || len(code) > 64 {
		return ""
	}

	return args[0]
}

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
					Approved: false,
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

			if err := s.EnsureInviteCode(ctx, u); err != nil {
				return err
			}
			if err := s.EnsureDiglogID(ctx, u); err != nil {
				return err
			}

			if !u.Approved {
				code := extractInviteCode(c)
				if code == "" {
					return ErrNotApproved
				}

				if err := s.CheckInviteCode(ctx, u, code); err != nil {
					return err
				}
				if !u.Approved {
					return ErrNotApproved
				}
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
				return c.Send(loc.ErrNotApproved())
			case errors.Is(err, ErrContextTooLong):
				return c.Send(
					loc.ErrContextTooLongMessage(),
					ybot.SingleButtonMenu("reset_chat_context", loc.ResetInlineButton()),
				)
			default:
				return c.Send(loc.ErrDefaultMessage())
			}
		}
	}
}
