package genoa

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// Run is the main entry point for the genoa command.
func Run(ctx context.Context, c *cli.Command) error {
	fmt.Println(Version(false))
	return nil
}
