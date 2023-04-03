package jeepity

import (
	"github.com/mkuznets/telebot/v3"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/jeepity/internal/ybot"
)

const ctxKeyUser = "user"

func Authenticate(s store.Store) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			ctx := ybot.Ctx(c)

			u, err := s.GetUser(ctx, c.Message().Chat.ID)
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
				if err := s.PutUser(ctx, u); err != nil {
					return err
				}
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

			switch err {
			case ErrNotApproved:
				return c.Send("⛔️ Вы не можете использовать этот бот")
			case ErrContextTooLong:
				return c.Send("⛔️ В текущем диалоге сликом много сообщений", resetMenu)
			default:
				return c.Send("❌ Что-то пошло не так. Пожалуйста, попробуйте еще раз")
			}
		}
	}
}
