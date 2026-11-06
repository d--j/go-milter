//go:build !plan9

package milter

import (
	"syscall"
	"testing"
)

func TestServer_AcceptSystemFileLimit(t *testing.T) {
	testServerAcceptResourceError(t, syscall.ENFILE)
}
