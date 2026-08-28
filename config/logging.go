package config

import (
	"log/slog"
	"os"

	"go.rtnl.ai/x/rlog"
	"go.rtnl.ai/x/rlog/console"
)

func (c Config) SetupLogging() {
	// Set the logging level.
	rlog.SetLevel(c.GetLogLevel())
	opts := &console.Options{
		HandlerOptions: rlog.MergeWithCustomLevels(rlog.WithGlobalLevel(nil)),
	}

	var stdout slog.Handler
	if c.ConsoleLog {
		stdout = console.New(os.Stdout, opts)
	} else {
		stdout = slog.NewJSONHandler(os.Stdout, opts.HandlerOptions)
	}

	rlog.SetDefault(rlog.New(slog.New(stdout)))
}
