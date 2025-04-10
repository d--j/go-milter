package main

import (
	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/integration"
	"log"
)

type ExampleBackend struct {
	milter.NoOpMilter
	from string
}

func (b *ExampleBackend) MailFrom(from string, _ string, m milter.Modifier) (*milter.Response, error) {
	b.from = from
	log.Printf("[%d] milter: mail from: %s", m.MilterId(), from)
	return milter.RespContinue, nil
}

func (b *ExampleBackend) RcptTo(rcptTo string, _ string, m milter.Modifier) (*milter.Response, error) {
	log.Printf("[%d] milter: rcpt to: %s", m.MilterId(), rcptTo)
	// reject the mail when it goes to other-spammer@example.com
	if rcptTo == "other-spammer@example.com" {
		return milter.RejectWithCodeAndReason(550, "5.7.1 Rejected by example backend")
	}
	return milter.RespContinue, nil
}

func (b *ExampleBackend) EndOfMessage(_ milter.Modifier) (*milter.Response, error) {
	if b.from == "reject-me@example.com" {
		return milter.RejectWithCodeAndReason(550, "5.7.1 We do not like you\nvery much, please go away")
	}
	return milter.RespAccept, nil
}

var _ milter.Milter = (*ExampleBackend)(nil)

func main() {
	integration.TestServer(milter.WithMilter(func() milter.Milter {
		return &ExampleBackend{}
	}))
}
