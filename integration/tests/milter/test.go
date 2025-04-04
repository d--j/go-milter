package main

import (
	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/milterutil"
	"log"
)

type ExampleBackend struct {
	milter.NoOpMilter
	from string
}

func (b *ExampleBackend) MailFrom(from string, _ string, _ *milter.Modifier) (*milter.Response, error) {
	b.from = from
	return milter.RespContinue, nil
}

func (b *ExampleBackend) RcptTo(rcptTo string, _ string, _ *milter.Modifier) (*milter.Response, error) {
	// reject the mail when it goes to other-spammer@example.com
	if rcptTo == "other-spammer@example.com" {
		return milter.RejectWithCodeAndReason(550, "5.7.1 Rejected by example backend")
	}
	return milter.RespContinue, nil
}

func (b *ExampleBackend) EndOfMessage(_ *milter.Modifier) (*milter.Response, error) {
	if b.from == "reject-me@example.com" {
		raw, err := milterutil.FormatResponse(550, "5.7.1 We do not like you\nvery much, please go away")
		if err != nil {
			panic(err)
		}
		log.Printf("Rejecting message from %s with raw response: %q", b.from, raw)
		return milter.RejectWithCodeAndReason(550, "5.7.1 We do not like you\nvery much, please go away")
	}
	return milter.RespAccept, nil
}

func main() {
	integration.TestServer(milter.WithMilter(func() milter.Milter {
		return &ExampleBackend{}
	}))
}
