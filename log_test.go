package milter

import (
	"bytes"
	"log"
	"testing"
)

func TestLogWarning(t *testing.T) {
	var buf bytes.Buffer
	origOut, origFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	}()

	logWarning("something %s: %d", "failed", 42)

	if got, want := buf.String(), "milter: warning: something failed: 42\n"; got != want {
		t.Fatalf("logWarning() wrote %q, want %q", got, want)
	}
}
