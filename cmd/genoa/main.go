package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"
	"go.rtnl.ai/genoa"
)

func main() {
	_ = godotenv.Load()

	app := &cli.Command{
		Name:    "genoa",
		Usage:   "runs ahead of rotational apps to ensure the environment is ready",
		Version: genoa.Version(false),
		Action:  genoa.Run,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		if ec, ok := err.(cli.ExitCoder); ok {
			os.Exit(ec.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
