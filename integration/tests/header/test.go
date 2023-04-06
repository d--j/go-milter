package main

import (
	"context"
	"io"
	"log"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
	"github.com/emersion/go-message/mail"
)

func main() {
	integration.Test(func(ctx context.Context, trx mailfilter.Trx) (mailfilter.Decision, error) {
		switch trx.MailFrom().Addr {
		case "add@example.com":
			trx.Headers().Add("X-ADD1", "Test")
			trx.Headers().Add("X-ADD2", "Test")
		case "add-first@example.com":
			f := trx.Headers().Fields()
			f.Next()
			f.InsertBefore("X-First1", "Test")
			f.InsertBefore("X-First2", "Test")
		case "add-middle@example.com":
			f := trx.Headers().Fields()
			for f.Next() {
				if f.CanonicalKey() == "Subject" {
					f.InsertBefore("X-Middle1", "Test")
					f.InsertBefore("X-Middle2", "Test")
					break
				}
			}
		case "subject@example.com":
			trx.Headers().SetSubject("changed")
		case "del@example.com":
			f := trx.Headers().Fields()
			for f.Next() {
				if f.CanonicalKey() == "Subject" {
					f.Del()
					break
				}
			}
		case "multi@example.com":
			f := trx.Headers().Fields()
			first := true
			for f.Next() {
				if first {
					f.InsertBefore("X-First1", "Test")
					f.InsertBefore("X-First2", "Test")
					first = false
				}
				if f.CanonicalKey() == "Subject" {
					f.Del()
				}
				if f.CanonicalKey() == "Date" {
					f.InsertBefore("X-Before-DATE", "Test")
				}
			}
			trx.Headers().Add("X-ADD1", "Test")
			trx.Headers().Add("X-ADD2", "Test")
		case "change-to@example.com":
			addr, err := trx.Headers().AddressList("To")
			if err != nil {
				return nil, err
			}
			addr = append(addr, &mail.Address{Address: "to@example.org"})
			trx.Headers().SetAddressList("To", addr)
		default:
			return mailfilter.CustomErrorResponse(500, "unknown mail from"), nil
		}
		b, _ := io.ReadAll(trx.Headers().Reader())
		log.Printf("from %s header %q", trx.MailFrom().Addr, string(b))
		return mailfilter.Accept, nil
	})
}
