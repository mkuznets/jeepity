package ybot

import (
	"fmt"
	"sync"

	"github.com/mkuznets/telebot/v3"
)

func Sequential(keyFn func(c telebot.Context) string) telebot.MiddlewareFunc {
	var locks sync.Map

	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			key := keyFn(c)
			v, _ := locks.LoadOrStore(key, new(sync.Mutex))
			lock, ok := v.(*sync.Mutex)
			if !ok {
				panic(fmt.Sprintf("invalid type for key %q: %T", key, v))
			}

			lock.Lock()
			defer lock.Unlock()
			return next(c)
		}
	}
}
