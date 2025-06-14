# go-milter

[![GoDoc](https://godoc.org/github.com/d--j/go-milter?status.svg)](https://godoc.org/github.com/d--j/go-milter)
![Build status](https://github.com/d--j/go-milter/actions/workflows/go.yml/badge.svg?branch=main)
[![Coverage Status](https://coveralls.io/repos/github/d--j/go-milter/badge.svg?branch=main)](https://coveralls.io/github/d--j/go-milter?branch=main)

A Go library to write mail filters.

## Features

* With this library you can write both the client (MTA/SMTP-Server) and server (milter filter)
  in pure Go without sendmail's libmilter.
* Easy wrapper of the milter protocol that abstracts away many milter protocol quirks
  and lets you write mail filters with little effort.
* UTF-8 support
* IDNA support
* Client & Server support milter protocol version 6 with all features. E.g.:
  * all milter events including DATA, UNKNOWN, ABORT and QUIT NEW CONNECTION
  * milter can skip e.g. body chunks when it does not need all chunks
  * milter can send progress notifications when response can take some time 
  * milter can automatically instruct the MTA which macros it needs.
* Automatic [integration tests](integration/README.md) that test the compatibility with Postfix and Sendmail.

## Installation

```shell
go get -u github.com/d--j/go-milter
```

## Usage

The following example is a milter filter that:
* adds `[⚠️EXTERNAL] ` to the subject of all messages of unauthenticated users
* quarantines all messages sent to `spam-trap@スパム.example.com`
* rejects all messages sent from `スパム.example.com` domain (delayed by 5 seconds to slow down spammers)

See [GoDoc](https://godoc.org/github.com/d--j/go-milter/mailfilter) for more documentation and an example for a milter client or a raw milter server.

```go
package main

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

func main() {
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
```

## License

BSD 2-Clause

## Credits

Based on https://github.com/emersion/go-milter by [Simon Ser](https://github.com/emersion) which is based on https://github.com/phalaaxx/milter by
[Bozhin Zafirov](https://github.com/phalaaxx). [Max Mazurov](https://github.com/foxcpp) made major contributions to this code as well.
