package main

import (
	"context"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	integration.RequiredTags("auth-plain", "auth-no", "tls-starttls", "tls-no")
	integration.Test(func(ctx context.Context, trx mailfilter.Trx) (mailfilter.Decision, error) {
		if trx.Helo().TlsVersion == "" {
			return mailfilter.CustomErrorResponse(500, "No starttls"), nil
		}
		if trx.MailFrom().AuthenticatedUser() == "user1@example.com" {
			return mailfilter.CustomErrorResponse(502, "Ok"), nil
		}
		return mailfilter.CustomErrorResponse(501, "No authentication"), nil
	}, mailfilter.WithDecisionAt(mailfilter.DecisionAtMailFrom))
}
