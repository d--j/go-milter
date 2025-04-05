// Package integration has integration tests and utilities for integration tests.
package integration

import (
	"context"
	"flag"
	"fmt"
	"github.com/d--j/go-milter"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/d--j/go-milter/mailfilter"
	"golang.org/x/tools/go/buildutil"
)

var Network = flag.String("network", "", "network")
var Address = flag.String("address", "", "address")
var Tags []string

const ExitSkip = 99

func init() {
	flag.Var((*buildutil.TagsFlag)(&Tags), "tags", buildutil.TagsFlagDoc)
}

func Test(decider mailfilter.DecisionModificationFunc, opts ...mailfilter.Option) {
	if !flag.Parsed() {
		flag.Parse()
	}
	if Network == nil || *Network == "" {
		log.Fatal("no network specified")
	}
	if Address == nil || *Address == "" {
		log.Fatal("no address specified")
	}
	filter, err := mailfilter.New(*Network, *Address, decider, opts...)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Started milter on %s:%s", filter.Addr().Network(), filter.Addr().String())
	filter.Wait()
}

func TestServer(opts ...milter.Option) {
	if !flag.Parsed() {
		flag.Parse()
	}
	if Network == nil || *Network == "" {
		log.Fatal("no network specified")
	}
	if Address == nil || *Address == "" {
		log.Fatal("no address specified")
	}
	if len(opts) == 0 {
		log.Fatal("no options specified")
	}
	// create socket to listen on
	socket, err := net.Listen(*Network, *Address)
	if err != nil {
		log.Fatal(err)
	}
	defer socket.Close()

	server := milter.NewServer(opts...)

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

	wgDone.Wait()
}

func HasTag(tag string) bool {
	if !flag.Parsed() {
		flag.Parse()
	}
	for _, t := range Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func Skip(reason string) {
	log.Printf("skip test: %s", reason)
	os.Exit(ExitSkip)
}

func RequiredTags(tags ...string) {
	for _, t := range tags {
		if !HasTag(t) {
			Skip(fmt.Sprintf("required tags not met: %s", strings.Join(tags, ",")))
		}
	}
}
