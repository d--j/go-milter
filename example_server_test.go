package milter_test

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

func ExampleServer() {
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
		milter.WithMacroRequest(milter.StageRcpt, []milter.MacroName{milter.MacroRcptMailer}),
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
