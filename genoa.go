package genoa

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/urfave/cli/v3"
	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/endeavor"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/x/rlog"
	"go.rtnl.ai/x/rlog/console"
)

const (
	TTL = 10 * time.Second
)

// Run is the main entry point for the genoa command.
func Run(ctx context.Context, c *cli.Command) (err error) {
	// Load the configuration from the environment.
	var conf config.Config
	if conf, err = config.Get(); err != nil {
		return errors.Exit(errors.ExitConfig, err)
	}

	// Setup logging and begin the genoa initialization.
	SetupLogging(conf.GetLogLevel(), conf.ConsoleLog)
	rlog.Info("starting genoa initialization", slog.String("version", Version(false)))

	if conf.EnsureDatabase.Enabled {
		rlog.Info("ensuring endeavor database exists", slog.String("name", conf.EnsureDatabase.Name), slog.String("secret_name", conf.EnsureDatabase.SecretName))
		if err = endeavor.EnsureDatabase(ctx, conf); err != nil {
			return errors.Exit(errors.ExitFailure, err)
		}
	}

	rlog.Debug("genoa initialization completed successfully")
	return nil
}

func SetupLogging(level slog.Level, consoleLog bool) {
	rlog.SetLevel(level)
	opts := &console.Options{
		HandlerOptions: rlog.MergeWithCustomLevels(rlog.WithGlobalLevel(nil)),
	}

	var stdout slog.Handler
	if consoleLog {
		stdout = console.New(os.Stdout, opts)
	} else {
		stdout = slog.NewJSONHandler(os.Stdout, opts.HandlerOptions)
	}

	rlog.SetDefault(rlog.New(slog.New(stdout)))
}
