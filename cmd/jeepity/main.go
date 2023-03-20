package main

import (
	"github.com/joho/godotenv"
	"mkuznets.com/go/ytils/ycli"
	"mkuznets.com/go/ytils/ylog"
)

const (
	dbFilename = "jeepity.db"
)

// Global is a group of common flags for all subcommands.
type Global struct{}

type App struct {
	GlobalOpts *Global `group:"Global Options"`

	RunCmd *RunCommand `command:"run" description:"CommandStart the service"`
}

func init() {
	ylog.Setup()
}

func main() {
	_ = godotenv.Load()
	ycli.Main[App]()
}
