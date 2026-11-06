package milter

import (
	"errors"
	"syscall"
)

func isAcceptResourceError(err error) bool {
	return errors.Is(err, syscall.EMFILE)
}
