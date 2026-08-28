package genoa

import "context"

// A function that returns an zero valued Command object.
type Constructor func() Command

type Command interface {
	Kind() string
	Run(ctx context.Context) error
}
