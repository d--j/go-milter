//go:build !plan9

package milter

import (
	"errors"
	"syscall"
)

func isAcceptResourceError(err error) bool {
	return errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE)
}
