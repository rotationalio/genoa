package errors

import (
	k8serr "k8s.io/apimachinery/pkg/api/errors"
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
