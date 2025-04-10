package main

import (
	"log"
	"os"
)

var LevelOneLogger = log.New(os.Stdout, "= ", 0)
var LevelTwoLogger = log.New(os.Stdout, " = ", 0)
var LevelThreeLogger = log.New(os.Stdout, "  = ", 0)
var LevelFourLogger = log.New(os.Stdout, "   = ", 0)

func main() {
	config := ParseConfig()
	receiver := Receiver{Config: config}
	if err := receiver.Start(); err != nil {
		config.Cleanup()
		LevelOneLogger.Fatal(err)
	}
	runner := NewRunner(config, &receiver)
	exitCode := 0
	if !runner.Run() {
		exitCode = 1
	}
	receiver.Cleanup()
	config.Cleanup()
	os.Exit(exitCode)
}
