package main

import (
	"log"
	"os"
)

var LevelOneLogger = log.New(os.Stdout, "= ", 0)
var LevelTwoLogger = log.New(os.Stdout, "== ", 0)
var LevelThreeLogger = log.New(os.Stdout, "=== ", 0)

func main() {
	config := ParseConfig()
	defer config.Cleanup()
	receiver := Receiver{Config: config}
	if err := receiver.Start(); err != nil {
		LevelOneLogger.Fatal(err)
	}
	defer receiver.Cleanup()
	runner := NewRunner(config, &receiver)
	if !runner.Run() {
		receiver.Cleanup()
		config.Cleanup()
		os.Exit(1)
	}
}
