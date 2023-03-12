// Package integration has integration tests and utilities for integration tests.
package integration

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

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
