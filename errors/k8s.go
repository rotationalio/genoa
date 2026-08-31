package errors

import (
	"errors"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
)

var (
	ErrMissingSecretName = errors.New("secret name is required")
	ErrMissingSecretKey  = errors.New("secret key is required")
)

func Wrap(err error) error {
	if err == nil {
		return nil
	}

	if k8serr.IsNotFound(err) {
		return Join(ErrNotFound, err)
	}

	return err
}
