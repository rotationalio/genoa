package genoa

import (
	"context"

	"go.rtnl.ai/genoa/config"
)

// A function that returns an zero valued Command object.
type Constructor func() Command

type Command interface {
	Kind() string
	Run(context.Context, config.Config) error
}
