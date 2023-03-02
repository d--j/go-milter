package milter

import (
	"fmt"
	"log"
)

func logWarning(format string, v ...interface{}) {
	log.Printf(fmt.Sprintf("milter: warning: %s", format), v...)
}

// LogWarning is called by this library when it wants to output a warning.
// Warnings can happen even when the library user did everything right (because the other end did something wrong)
//
// The default implementation uses [log.Print] to output the warning.
// You can re-assign LogWarning to something more suitable for your application. But do not assign nil to it.
var LogWarning = logWarning
