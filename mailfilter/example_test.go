package mailfilter_test

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/d--j/go-milter/mailfilter"
)

func ExampleNew() {
	// parse commandline arguments
	var protocol, address string
	flag.StringVar(&protocol, "proto", "tcp", "Protocol family (unix or tcp)")
	flag.StringVar(&address, "addr", "127.0.0.1:10003", "Bind to address or unix domain socket")
	flag.Parse()

	// create and start the mail filter
	mailFilter, err := mailfilter.New(protocol, address,
		func(_ context.Context, trx *mailfilter.Transaction) (mailfilter.Decision, error) {
			// Reject message when it was sent to our SPAM trap
			if trx.HasRcptTo("spam-trap@スパム.example.com") {
				return mailfilter.CustomErrorResponse(550, "5.7.1 No thank you"), nil
			}
			// Prefix subject with [⚠️EXTERNAL] when user is not logged in
			if trx.MailFrom.AuthenticatedUser() == "" {
				subject, _ := trx.Headers.Subject()
				if !strings.HasPrefix(subject, "[⚠️EXTERNAL] ") {
					subject = "[⚠️EXTERNAL] " + subject
				}
				trx.Headers.SetSubject(subject)
			}
			return mailfilter.Accept, nil
		},
		// optimization: we do not need the body of the message for our decision
		mailfilter.WithoutBody(),
	)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Started milter on %s:%s", mailFilter.Addr().Network(), mailFilter.Addr().String())

	// wait for the mail filter to end
	mailFilter.Wait()
}
