package main

import (
	"context"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	integration.Test(
		func(ctx context.Context, trx mailfilter.Trx) (mailfilter.Decision, error) {
			if trx.HasRcptTo("temp-fail@example.com") {
				return mailfilter.TempFail, nil
			}
			if trx.HasRcptTo("reject@example.com") {
				return mailfilter.Reject, nil
			}
			if trx.HasRcptTo("discard@example.com") {
				return mailfilter.Discard, nil
			}
			if trx.HasRcptTo("custom@example.com") {
				return mailfilter.CustomErrorResponse(555, "5.0.0 custom"), nil
			}
			if trx.HasRcptTo("quarantine@example.com") {
				return mailfilter.QuarantineResponse("test"), nil
			}
			if trx.HasRcptTo("add@example.com") {
				trx.AddRcptTo("another@example.com", "")
			}
			if trx.HasRcptTo("change@example.com") {
				trx.DelRcptTo("change@example.com")
				// Sendmail does not like setting ESMTP args, so we do not set any
				trx.AddRcptTo("another@example.com", "")
			}
			return mailfilter.Accept, nil
			// the decision is done at the DATA command but the mock server sends the DATA response before we can intercept
			// so the testcases allow the rejection at any step, not only at "DATA".
		},
		mailfilter.WithDecisionAt(mailfilter.DecisionAtData),
		mailfilter.WithRcptToValidator(func(_ context.Context, in *mailfilter.RcptToValidationInput) (mailfilter.Decision, error) {
			if in.RcptTo.Addr == "reject-one@example.com" {
				return mailfilter.Reject, nil
			}
			if in.RcptTo.Addr == "discard-one@example.com" {
				return mailfilter.Discard, nil // discards the whole transaction, not only this recipient
			}
			return mailfilter.Accept, nil
		}),
	)
}
