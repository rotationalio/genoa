package errors

import (
	"github.com/urfave/cli/v3"
)

const (
	ExitSuccess int = iota
	ExitFailure
	ExitConfig
)

func Exit(code int, err error) error {
	return ExitError{
		Code: code,
		Err:  err,
	}
}

type ExitError struct {
	Code int
	Err  error
}

var _ cli.ExitCoder = (*ExitError)(nil)

func (e ExitError) Error() string {
	return e.Err.Error()
}

func (e ExitError) ExitCode() int {
	return e.Code
}

func (e ExitError) Unwrap() error {
	return e.Err
}
