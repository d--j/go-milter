package main

import (
	"context"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	integration.Test(func(ctx context.Context, trx mailfilter.Trx) (mailfilter.Decision, error) {
		if trx.MTA().FQDN == "" {
			return mailfilter.CustomErrorResponse(500, "no mta"), nil
		}
		return mailfilter.Accept, nil
	})
}
