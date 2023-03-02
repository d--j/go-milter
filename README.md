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

## Installation

```shell
go get -u github.com/d--j/go-milter
```

## Usage

The following example is a milter filter that adds `[⚠️EXTERNAL] ` to the subject of all messages of unauthenticated users.

See [GoDoc](https://godoc.org/github.com/d--j/go-milter/mailfilter) for more documentation and an example for a milter client or a raw milter server.

```go
package main

import (
  "context"
  "flag"
  "log"
  "strings"

  "github.com/d--j/go-milter/mailfilter"
)

func main() {
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
```

## License

BSD 2-Clause

## Credits

Based on https://github.com/emersion/go-milter by [Simon Ser](https://github.com/emersion) which is based on https://github.com/phalaaxx/milter by
[Bozhin Zafirov](https://github.com/phalaaxx). [Max Mazurov](https://github.com/foxcpp) made major contributions to this code as well.
