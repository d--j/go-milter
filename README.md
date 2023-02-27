# go-milter

[![GoDoc](https://godoc.org/github.com/d--j/go-milter?status.svg)](https://godoc.org/github.com/d--j/go-milter)
![Build status](https://github.com/d--j/go-milter/actions/workflows/go.yml/badge.svg?branch=main)
[![Coverage Status](https://coveralls.io/repos/github/d--j/go-milter/badge.svg?branch=main)](https://coveralls.io/github/d--j/go-milter?branch=main)

A Go library to write mail filters.

With this library you can write both the client (MTA/SMTP-Server) and server (milter filter)
in pure Go without sendmail's libmilter.

## Features

* Client & Server support milter protocol version 6 with all features. E.g.:
  * all milter events including DATA, UNKNOWN, ABORT and QUIT NEW CONNECTION
  * milter can skip e.g. body chunks when it does not need all chunks
  * milter can send progress notifications when response can take some time 
  * milter can automatically instruct the MTA which macros it needs.
* UTF-8 support

## Usage

```go
package main

import (
	"log"
	"net"
	"sync"

	"github.com/d--j/go-milter"
)

type ExampleBackend struct {
	milter.NoOpMilter
}

func (b *ExampleBackend) RcptTo(rcptTo string, esmtpArgs string, m *milter.Modifier) (*milter.Response, error) {
	// reject the mail when it goes to other-spammer@example.com and is a local delivery
	if rcptTo == "other-spammer@example.com" && m.Macros.Get(milter.MacroRcptMailer) == "local" {
		return milter.RejectWithCodeAndReason(550, "We do not like you\r\nvery much, please go away")
	}
	return milter.RespContinue, nil
}

func main() {
	// create socket to listen on
	socket, err := net.Listen("tcp4", "127.0.0.1:6785")
	if err != nil {
		log.Fatal(err)
	}
	defer socket.Close()

	// define the backend, required actions, protocol options and macros we want
	server := milter.NewServer(
		milter.WithMilter(func() milter.Milter {
			return &ExampleBackend{}
		}),
		milter.WithProtocol(milter.OptNoConnect|milter.OptNoHelo|milter.OptNoMailFrom|milter.OptNoBody|milter.OptNoHeaders|milter.OptNoEOH|milter.OptNoUnknown|milter.OptNoData),
		milter.WithAction(milter.OptChangeFrom|milter.OptAddRcpt|milter.OptRemoveRcpt),
		milter.WithMaroRequest(milter.StageRcpt, []milter.MacroName{milter.MacroRcptMailer}),
	)
	defer server.Close()

	// start the milter
	var wgDone sync.WaitGroup
	wgDone.Add(1)
	go func(socket net.Listener) {
		if err := server.Serve(socket); err != nil {
			log.Fatal(err)
		}
		wgDone.Done()
	}(socket)

	log.Printf("Started milter on %s:%s", socket.Addr().Network(), socket.Addr().String())

	// quit when milter quits
	wgDone.Wait()
}
```

See [![GoDoc](https://godoc.org/github.com/d--j/go-milter?status.svg)](https://godoc.org/github.com/d--j/go-milter) for more documentation and an example for a milter client. 

## License

BSD 2-Clause

## Credits

Based on https://github.com/emersion/go-milter by [Simon Ser](https://github.com/emersion) which is based on https://github.com/phalaaxx/milter by
[Bozhin Zafirov](https://github.com/phalaaxx). [Max Mazurov](https://github.com/foxcpp) made major contributions to this code as well.
