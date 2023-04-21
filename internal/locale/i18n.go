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

var Bundle = i18n.NewBundle(language.English)

func init() {
	Bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	_, _ = Bundle.LoadMessageFileFS(localeFs, "locale.en.toml")
	_, _ = Bundle.LoadMessageFileFS(localeFs, "locale.ru.toml")
}

func New(lang string) *i18n.Localizer {
	return i18n.NewLocalizer(Bundle, lang, "en")
}

func M(loc *i18n.Localizer, m *i18n.Message) string {
	return y.Must(loc.LocalizeMessage(m))
}
