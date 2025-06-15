package mailfilter_test

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/d--j/go-milter/mailfilter"
)

func ExampleNew() {
	// parse commandline arguments
	var protocol, address string
	flag.StringVar(&protocol, "proto", "tcp", "Protocol family (unix or tcp)")
	flag.StringVar(&address, "addr", "127.0.0.1:10003", "Bind to address or unix domain socket")
	flag.Parse()

	// create and start the mail filter
	filter, err := mailfilter.New(protocol, address,
		func(_ context.Context, trx mailfilter.Trx) (mailfilter.Decision, error) {
			// Quarantine mail when it is addressed to our SPAM trap
			if trx.HasRcptTo("spam-trap@スパム.example.com") {
				return mailfilter.QuarantineResponse("train as spam"), nil
			}
			// Prefix Subject with [⚠️EXTERNAL] when the user is not logged in
			if trx.MailFrom().AuthenticatedUser() == "" {
				subject, _ := trx.Headers().Subject()
				if !strings.HasPrefix(subject, "[⚠️EXTERNAL] ") {
					subject = "[⚠️EXTERNAL] " + subject
					trx.Headers().SetSubject(subject)
				}
			}
			return mailfilter.Accept, nil
		},
		mailfilter.WithRcptToValidator(func(_ context.Context, in *mailfilter.RcptToValidationInput) (mailfilter.Decision, error) {
			if in.MailFrom.UnicodeDomain() == "スパム.example.com" {
				time.Sleep(time.Second * 5) // slow down the spammer
				return mailfilter.CustomErrorResponse(554, "5.7.1 You cannot send from this domain"), nil
			}
			return mailfilter.Accept, nil
		}),
		// Optimization: call the decision function when all headers were sent to us. Modifications get automatically deferred to EndOfHeaders.
		mailfilter.WithDecisionAt(mailfilter.DecisionAtEndOfHeaders),
	)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Started milter on %s:%s", filter.Addr().Network(), filter.Addr().String())

	// wait for SIGINT or SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("Gracefully shutting down milter…")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		filter.Shutdown(ctx)
	}()
	// wait for the mail filter to end
	filter.Wait()
}
