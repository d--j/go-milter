package main

import (
	"context"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	// The Postfix test configuration uses milter_default_action = reject.
	integration.RequiredTags("mta-postfix")
	integration.Test(
		func(context.Context, mailfilter.Trx) (mailfilter.Decision, error) {
			return mailfilter.Accept, nil
		},
		mailfilter.WithRcptToValidator(func(_ context.Context, in *mailfilter.RcptToValidationInput) (mailfilter.Decision, error) {
			if in.RcptTo.Addr == "panic@example.com" {
				panic("recipient validator panic")
			}
			return mailfilter.Accept, nil
		}),
	)
}
