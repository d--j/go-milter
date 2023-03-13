package main

import (
	"context"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	integration.Test(func(ctx context.Context, trx *mailfilter.Transaction) (mailfilter.Decision, error) {
		if trx.MailFrom.Addr == "temp-fail@example.com" {
			return mailfilter.TempFail, nil
		}
		if trx.MailFrom.Addr == "reject@example.com" {
			return mailfilter.Reject, nil
		}
		if trx.MailFrom.Addr == "discard@example.com" {
			return mailfilter.Discard, nil
		}
		if trx.MailFrom.Addr == "custom@example.com" {
			return mailfilter.CustomErrorResponse(555, "custom"), nil
		}
		if trx.MailFrom.Addr == "quarantine@example.com" {
			return mailfilter.QuarantineResponse("test"), nil
		}
		if trx.MailFrom.Addr == "change@example.com" {
			trx.MailFrom.Addr = "another@example.com"
			// Sendmail might break when you do not null this out
			if trx.MTA.IsSendmail() {
				trx.MailFrom.Args = ""
			}
		}
		return mailfilter.Accept, nil
	}, mailfilter.WithDecisionAt(mailfilter.DecisionAtMailFrom))
}
