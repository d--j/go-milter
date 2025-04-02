// Command milter-check can be used to send test data to milters.
package main

import (
	"bufio"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/milterutil"
	"github.com/emersion/go-message/textproto"
	"golang.org/x/text/transform"
)

func printAction(prefix string, act *milter.Action) {
	switch act.Type {
	case milter.ActionAccept:
		log.Println(prefix, "accept")
	case milter.ActionReject:
		log.Println(prefix, "reject")
	case milter.ActionDiscard:
		log.Println(prefix, "discard")
	case milter.ActionTempFail:
		log.Println(prefix, "temp. fail")
	case milter.ActionRejectWithCode:
		log.Println(prefix, "reply code:", act.SMTPCode, act.SMTPReply)
	case milter.ActionContinue:
		log.Println(prefix, "continue")
	case milter.ActionSkip:
		log.Println(prefix, "skip")
	}
}

func printModifyAction(act milter.ModifyAction) {
	switch act.Type {
	case milter.ActionAddHeader:
		log.Printf("add header: name %s, value %s", act.HeaderName, act.HeaderValue)
	case milter.ActionInsertHeader:
		log.Printf("insert header: at %d, name %s, value %s", act.HeaderIndex, act.HeaderName, act.HeaderValue)
	case milter.ActionChangeFrom:
		log.Printf("change from: %s %v", act.From, act.FromArgs)
	case milter.ActionChangeHeader:
		log.Printf("change header: at %d, name %s, value %s", act.HeaderIndex, act.HeaderName, act.HeaderValue)
	case milter.ActionReplaceBody:
		log.Println("replace body:", string(act.Body))
	case milter.ActionAddRcpt:
		log.Println("add rcpt:", act.Rcpt)
	case milter.ActionDelRcpt:
		log.Println("del rcpt:", act.Rcpt)
	case milter.ActionQuarantine:
		log.Println("quarantine:", act.Reason)
	}
}

func main() {
	transport := flag.String("transport", "unix", "Transport to use for milter connection, One of 'tcp', 'unix', 'tcp4' or 'tcp6'")
	address := flag.String("address", "", "Transport address, path for 'unix', address:port for 'tcp'")
	hostname := flag.String("hostname", "localhost", "Value to send in CONNECT message")
	family := flag.String("family", string(milter.FamilyInet), "Protocol family to send in CONNECT message")
	port := flag.Uint("port", 2525, "Port to send in CONNECT message")
	connAddr := flag.String("conn-addr", "127.0.0.1", "Connection address to send in CONNECT message")
	helo := flag.String("helo", "localhost", "Value to send in HELO message")
	mailFrom := flag.String("from", "foxcpp@example.org", "Value to send in MAIL message")
	rcptTo := flag.String("rcpt", "foxcpp@example.com", "Comma-separated list of values for RCPT messages")
	actionMask := flag.Uint("actions",
		uint(milter.AllClientSupportedActionMasks),
		"Bitmask value of actions we allow")
	disabledMsgs := flag.Uint("disabled-msgs", 0, "Bitmask of disabled protocol messages")
	flag.Parse()

	c := milter.NewClient(*transport, *address, milter.WithActions(milter.OptAction(*actionMask)), milter.WithProtocols(milter.OptProtocol(*disabledMsgs)))

	s, err := c.Session(nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer func(s *milter.ClientSession) {
		_ = s.Close()
	}(s)

	act, err := s.Conn(*hostname, milter.ProtoFamily((*family)[0]), uint16(*port), *connAddr)
	if err != nil {
		log.Println(err)
		return
	}
	printAction("CONNECT:", act)
	if act.StopProcessing() {
		return
	}

	act, err = s.Helo(*helo)
	if err != nil {
		log.Println(err)
		return
	}
	printAction("HELO:", act)
	if act.StopProcessing() {
		return
	}

	act, err = s.Mail(*mailFrom, "")
	if err != nil {
		log.Println(err)
		return
	}
	printAction("MAIL:", act)
	if act.StopProcessing() {
		return
	}

	for _, rcpt := range strings.Split(*rcptTo, ",") {
		act, err = s.Rcpt(rcpt, "")
		if err != nil {
			log.Println(err)
			return
		}
		switch act.Type {
		case milter.ActionAccept:
			log.Println("RCPT: accept recipient", rcpt)
		case milter.ActionReject:
			log.Println("RCPT: reject recipient", rcpt)
		case milter.ActionDiscard:
			log.Println("RCPT: discard")
			return
		case milter.ActionTempFail:
			log.Println("RCPT: temp. fail recipient", rcpt)
		case milter.ActionRejectWithCode:
			log.Println("RCPT: reply code reject recipient:", act.SMTPCode, act.SMTPReply, rcpt)
		case milter.ActionContinue:
			log.Println("RCPT: accept recipient (continue)", rcpt)
		case milter.ActionSkip:
			log.Println("RCPT: accept recipient (skip)", rcpt)
		}
	}

	act, err = s.DataStart()
	if err != nil {
		log.Println(err)
		return
	}
	printAction("DATA:", act)
	if act.StopProcessing() {
		return
	}

	bufR := bufio.NewReader(transform.NewReader(os.Stdin, &milterutil.CrLfCanonicalizationTransformer{}))
	hdr, err := textproto.ReadHeader(bufR)
	if err != nil {
		log.Println("header parse:", err)
		return
	}

	act, err = s.Header(hdr)
	if err != nil {
		log.Println(err)
		return
	}
	printAction("HEADER:", act)
	if act.StopProcessing() {
		return
	}

	modifyActs, act, err := s.BodyReadFrom(bufR)
	if err != nil {
		log.Println(err)
		return
	}
	for _, act := range modifyActs {
		printModifyAction(act)
	}
	printAction("EOB:", act)
}
