// Command log-milter is a no-op milter that logs all milter communication
package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/d--j/go-milter"
)

//goland:noinspection SpellCheckingInspection
var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func main() {
	transport := flag.String("transport", "tcp", "Transport to use for milter connection, One of 'tcp', 'unix', 'tcp4' or 'tcp6'")
	address := flag.String("address", "127.0.0.1:0", "Transport address, path for 'unix', address:port for 'tcp'")

	flag.Parse()

	// make sure socket does not exist
	if *transport == "unix" {
		// ignore os.Remove errors
		_ = os.Remove(*address)
	}
	// bind to listening address
	socket, err := net.Listen(*transport, *address)
	if err != nil {
		log.Fatal(err)
	}
	defer func(socket net.Listener) {
		_ = socket.Close()
	}(socket)

	if *transport == "unix" {
		// set mode 0660 for unix domain sockets
		if err := os.Chmod(*address, 0660); err != nil {
			log.Fatal(err)
		}
		// remove socket on exit
		defer func(name string) {
			_ = os.Remove(name)
		}(*address)
	}

	server := milter.NewServer(
		milter.WithMilter(func() milter.Milter {
			return &LogMilter{logPrefix: randSeq(10)}
		}),
		milter.WithNegotiationCallback(func(mtaVersion, milterVersion uint32, mtaActions, milterActions milter.OptAction, mtaProtocol, milterProtocol milter.OptProtocol, offeredDataSize milter.DataSize) (version uint32, actions milter.OptAction, protocol milter.OptProtocol, maxDataSize milter.DataSize, err error) {
			log.Printf("ACCEPT milter version %d, actions %032b, protocol %032b, data size %d", mtaVersion, mtaActions, mtaProtocol, offeredDataSize)
			return mtaVersion, mtaActions, 0, offeredDataSize, nil
		}),
	)

	defer func(server *milter.Server) {
		_ = server.Close()
	}(server)
	var wgDone sync.WaitGroup
	wgDone.Add(1)
	go func(socket net.Listener) {
		if err := server.Serve(socket); err != nil {
			log.Println(err)
		}
		wgDone.Done()
	}(socket)

	log.Printf("Started milter on %s:%s", socket.Addr().Network(), socket.Addr().String())

	// wait for SIGINT or SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("Gracefully shutting down milter…")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	// quit when milter quits
	wgDone.Wait()
}
