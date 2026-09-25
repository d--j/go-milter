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

## Deployment considerations

### Restrict access to the milter socket

Only trusted MTAs should be able to connect to the milter socket. Milter protocol
negotiation exchanges capabilities; it does not authenticate the connecting MTA.
SMTP authentication information, such as `AuthenticatedUser()` in the example
above, describes the mail sender and does not control access to the milter socket.

For a local deployment, use a Unix domain socket with permissions on the socket
and its parent directory that restrict access to the intended services, or bind
TCP to a loopback address such as `127.0.0.1`. Loopback prevents remote access but
does not restrict which local processes can connect. If the MTA connects over a
network, restrict access to the intended MTA hosts with firewall rules or
equivalent network controls. Do not expose the milter socket to the public Internet.

### Choose how the MTA handles filter failures

If the filter is unavailable or its connection fails, the MTA's configuration
determines what happens to affected mail. For Postfix, review
[`milter_default_action`](https://www.postfix.org/postconf.5.html#milter_default_action)
in your deployment. `shutdown` disconnects the SMTP client; `tempfail` temporarily
rejects further commands in the SMTP session. Postfix 3.11 changed the default from
`tempfail` to `shutdown`, and the change was also
[backported to earlier stable releases](https://www.postfix.org/announcements/postfix-3.10.8.html).
Check the effective setting with `postconf milter_default_action`. `reject` rejects
further commands with a permanent error, `accept` continues without that filter,
and `quarantine` also continues but places the message on hold. Choose the behavior
appropriate for your filtering requirements.

`mailfilter.WithErrorHandling` selects the response sent when an application
callback returns an error. go-milter then closes the milter connection, so the
MTA's failure handling applies to the rest of that SMTP session. With
`mailfilter.Error`, or when a callback panics, no response is sent and the MTA's
failure handling applies immediately.

### Keep deployed applications updated

Check the project's [security advisories](https://github.com/d--j/go-milter/security/advisories)
for affected versions and fixes. After updating the go-milter dependency, rebuild
and redeploy applications that use it so running services include the fixes.

## License

BSD 2-Clause

## Credits

Based on https://github.com/emersion/go-milter by [Simon Ser](https://github.com/emersion) which is based on https://github.com/phalaaxx/milter by
[Bozhin Zafirov](https://github.com/phalaaxx). [Max Mazurov](https://github.com/foxcpp) made major contributions to this code as well.
