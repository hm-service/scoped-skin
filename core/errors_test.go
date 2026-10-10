package core_test

import (
	"errors"

	"scoped-skin/core"
)

func asValidation(err error, target **core.ValidationError) bool {
	return errors.As(err, target)
}

func asConflict(err error, target **core.ConflictError) bool {
	return errors.As(err, target)
}
