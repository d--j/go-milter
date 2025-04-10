package main

import (
	"errors"
	"time"

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

func (r *Runner) Run() bool {
	var prevMta *MTA
	var activeDir *TestDir
	defer func() {
		if activeDir != nil {
			activeDir.Stop()
		}
		if prevMta != nil {
			prevMta.Stop()
		}
	}()
	failedDir := false
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
				LevelTwoLogger.Printf("ERR starting MTA %v", err)
				return false
			}
		}
		activeDir = dir
		LevelTwoLogger.Print(dir.Path)
		if err := dir.Start(); err != nil {
			if errors.Is(err, ErrTestSkipped) {
				for _, t := range dir.Tests {
					i++
					LevelThreeLogger.Printf("%03d/%03d %s", i+1, tests, t.Filename)
					t.MarkSkipped("%03d/%03d SKIP", i, tests)
				}
				continue
			}
			LevelTwoLogger.Printf("ERR starting milter %v", err)
			return false
		}
		for _, t := range dir.Tests {
			i++
			LevelThreeLogger.Printf("%03d/%03d %s", i, tests, t.Filename)
			if !r.runTestCase(dir, t) {
				return false
			}
		}
		activeDir.Stop()
		if activeDir.failedTest {
			failedDir = true
		}
		activeDir = nil
	}
	if prevMta != nil {
		prevMta.Stop()
		prevMta = nil
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
		case TestReady:
			panic("test state is ready. This should never happen")
		}
	}
	LevelOneLogger.Printf("%d tests done: %d OK %d skipped %d failed", len(r.config.Tests), numOk, numSkipped, numFailed)
	return numFailed == 0 && !failedDir
}

func (r *Runner) runTestCase(dir *TestDir, t *TestCase) bool {
	if err := t.Connect(dir.MTA.Port); err != nil {
		t.MarkFailed("Connection error: %s", err.Error())
		return false
	}
	quit := func() {
		t.Quit()
		LevelFourLogger.Printf("QUIT")
	}
	type sendmailFallback struct {
		TransactionName string
		Diff            string
	}
	var sendmail []sendmailFallback
	LevelFourLogger.Printf("CONNECT")
	for i, transaction := range t.TestCase.Transactions {
		LevelFourLogger.Printf("%03d/%03d %s", i+1, len(t.TestCase.Transactions), transaction.Name)
		if i > 0 {
			time.Sleep(time.Second)
		}
		if transaction.ExpectsOutput() {
			r.receiver.ExpectMessage()
		} else {
			r.receiver.IgnoreMessages()
		}
		code, message, step, err := t.Send(transaction.InputSteps)
		if err != nil {
			quit()
			t.MarkFailed("ERR %v", err)
			return false
		}
		if !transaction.Decision.Compare(code, message, step) {
			r.receiver.IgnoreMessages()
			quit()
			t.MarkFailed("NOK DECISION expected %s got %d %q @%s", transaction.Decision, code, message, step)
			return true
		}
		if transaction.ExpectsOutput() {
			output := r.receiver.WaitForMessage()
			r.receiver.IgnoreMessages()
			diff, ok := integration.DiffOutput(transaction.Output, output)
			if !ok {
				if t.parent.MTA.HasTag("mta-sendmail") {
					if integration.CompareOutputSendmail(transaction.Output, output) {
						sendmail = append(sendmail, sendmailFallback{
							TransactionName: transaction.Name,
							Diff:            diff,
						})

						continue
					}
				}
				quit()
				t.MarkFailed("NOK OUTPUT %sRECEIVED OUTPUT\n%s", diff, output)
				return true
			}
		}
	}
	quit()
	if len(sendmail) > 0 {
		diff := ""
		for i, f := range sendmail {
			diff += f.TransactionName + "\n" + f.Diff
			if i < len(sendmail)-1 {
				diff += "\n"
			}
		}
		t.MarkOk("OK (sendmail) %s", diff)
	} else {
		t.MarkOk("OK")
	}
	return true
}
