package sqlite

import (
	"embed"
)

//go:generate atlas migrate diff update --dir "file://." --to "file://../schema.hcl" --dev-url "sqlite://file?mode=memory"

//go:embed *.sql
var FS embed.FS
