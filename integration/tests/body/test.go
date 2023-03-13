package main

import (
	"bytes"
	"context"
	"io"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	integration.Test(func(ctx context.Context, trx *mailfilter.Transaction) (mailfilter.Decision, error) {
		switch trx.MailFrom.Addr {
		case "add@example.com":
			b, err := io.ReadAll(trx.Body())
			if err != nil {
				return nil, err
			}
			b = append(b, "two\r\n"...)
			trx.ReplaceBody(bytes.NewReader(b))
		default:
			return mailfilter.CustomErrorResponse(500, "unknown mail from"), nil
		}
		return mailfilter.Accept, nil
	})
}
