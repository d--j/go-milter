//go:build: auth-no

package main

import (
	"context"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	integration.Test(func(ctx context.Context, trx *mailfilter.Transaction) (mailfilter.Decision, error) {
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
			return mailfilter.CustomErrorResponse(555, "custom"), nil
		}
		if trx.HasRcptTo("quarantine@example.com") {
			return mailfilter.QuarantineResponse("test"), nil
		}
		return mailfilter.Accept, nil
		// the decision is done at the DATA command but the mock server sends the DATA response before we can intercept
		// so the testcases allow the rejection at any step, not only at "DATA".
	}, mailfilter.WithDecisionAt(mailfilter.DecisionAtData))
}
