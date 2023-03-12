package main

import (
	"os"

	"github.com/d--j/go-milter/integration"
)

type Runner struct {
	config   *Config
	receiver *Receiver
}

func NewRunner(config *Config, receiver *Receiver) *Runner {
	return &Runner{
		config:   config,
		receiver: receiver,
	}
}

func (r *Runner) Run() {
	var prevMta *MTA
	var prevDir *TestDir
	defer func() {
		if prevDir != nil {
			prevDir.Stop()
		}
		if prevMta != nil {
			prevMta.Stop()
		}
	}()
	tests := len(r.config.Tests)
	i := 0
	for _, dir := range r.config.TestDirs {
		if prevMta != dir.MTA {
			if prevMta != nil {
				prevMta.Stop()
			}
			LevelOneLogger.Print(dir.MTA)
			prevMta = dir.MTA
			if err := dir.MTA.Start(); err != nil {
				LevelTwoLogger.Fatal(err)
			}
		}
		prevDir = dir
		LevelTwoLogger.Print(dir.Path)
		if err := dir.Start(); err != nil {
			if err == ErrTestSkipped {
				for _, t := range dir.Tests {
					i++
					LevelThreeLogger.Printf("%03d/%03d %s", i+1, tests, t.Filename)
					t.MarkSkipped("%03d/%03d SKIP", i, tests)
				}
				continue
			}
			LevelTwoLogger.Fatal(err)
		}
		for _, t := range dir.Tests {
			i++
			LevelThreeLogger.Printf("%03d/%03d %s", i, tests, t.Filename)
			if t.TestCase.ExpectsOutput() {
				r.receiver.ExpectMessage()
			}
			code, message, step, err := t.Send(t.TestCase.InputSteps, dir.MTA.Port)
			if err != nil {
				prevMta.MarkFailedTest()
				prevMta.Stop()
				LevelThreeLogger.Fatal(err)
			}
			if !t.TestCase.Decision.Compare(code, message, step) {
				r.receiver.IgnoreMessages()
				t.MarkFailed("%03d/%03d NOK DECISION %s != %d %s @%s", i, tests, t.TestCase.Decision, code, message, step)
				continue
			}
			if t.TestCase.ExpectsOutput() {
				output := r.receiver.WaitForMessage()
				r.receiver.IgnoreMessages()
				diff, ok := integration.DiffOutput(t.TestCase.Output, output)
				if !ok {
					if t.parent.MTA.HasTag("mta-sendmail") {
						if integration.CompareOutputSendmail(t.TestCase.Output, output) {
							t.MarkOk("%03d/%03d OK (sendmail) %s", i, tests, diff)
							continue
						}
					}
					t.MarkFailed("%03d/%03d NOK OUTPUT %s", i, tests, diff)
					continue
				}
			}
			t.MarkOk("%03d/%03d OK", i, tests)
		}
		prevDir.Stop()
	}
	numOk, numSkipped, numFailed := 0, 0, 0
	for _, t := range r.config.Tests {
		switch t.State {
		case TestOk:
			numOk++
		case TestSkipped:
			numSkipped++
		case TestFailed:
			numFailed++
		}
	}
	LevelOneLogger.Printf("%d tests done: %d OK %d skipped %d failed", len(r.config.Tests), numOk, numSkipped, numFailed)
	if numFailed > 0 {
		os.Exit(1)
	}
}
