package errors

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound           = errors.New("object not found")
	ErrMissingDatabaseURL = errors.New("admin database url is required for this operation")
	ErrInvalidCreateDSN   = errors.New("dsn requires database name, username, and password")
	ErrInvalidName        = errors.New("name is not valid for this operation")
	ErrInvalidVersion     = errors.New("version is not valid")
	ErrUnknownCommand     = errors.New("unknown command kind")
	ErrInvalidArgs        = errors.New("command args cannot be unmarshaled")
	ErrAlreadyRegistered  = errors.New("command already registered")
)

// In is a helper function to check if an error is in a list of errors.
func In(err error, targets ...error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// Reduce namespacing conflicts by adding error functions from the errors package.
var (
	New    = errors.New
	Fmt    = fmt.Errorf
	Is     = errors.Is
	As     = errors.As
	Join   = errors.Join
	Unwrap = errors.Unwrap
)
