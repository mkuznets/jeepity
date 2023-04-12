package jeepity

import (
	"errors"
	"fmt"

	"github.com/mkuznets/telebot/v3"

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

func ErrorHandler(resetMenu *telebot.ReplyMarkup) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			err := next(c)
			if err == nil {
				return nil
			}

			switch {
			case errors.Is(err, ErrNotApproved):
				return c.Send("⛔️ Вы не можете использовать этот бот")
			case errors.Is(err, ErrContextTooLong):
				return c.Send("⛔️ В текущем диалоге сликом много сообщений", resetMenu)
			default:
				return c.Send("❌ Что-то пошло не так. Пожалуйста, попробуйте еще раз")
			}
		}
	}
}
