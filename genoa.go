package genoa

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/genoa/k8s"
)

const (
	TTL = 10 * time.Second
)

// Run is the main entry point for the genoa command.
func Run(ctx context.Context, c *cli.Command) error {
	fmt.Println(Version(false))
	secrets, err := k8s.FetchSecret(ctx, "agenix-endeavor-foo")
	if err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			fmt.Println("secret not found, creating")
			return nil
		}
		return err
	}

	for key, val := range secrets {
		fmt.Println(key, string(val))
	}
	return nil
}
