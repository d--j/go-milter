package milter_test

import (
	"log"
	"strings"
	"time"

	"github.com/d--j/go-milter"
)

func ExampleClient() {
	// create milter definition once
	client := milter.NewClient("tcp", "127.0.0.1:1234")
	globalMacros := milter.NewMacroBag()
	globalMacros.Set(milter.MacroMTAFQDN, "localhost.local")
	globalMacros.Set(milter.MacroMTAPid, "123")

	// on each SMTP connection
	macros := globalMacros.Copy()
	session, err := client.Session(macros)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	handleMilterResponse := func(act *milter.Action, err error) {
		if err != nil {
			// you should disable this milter for this connection or close the SMTP transaction
			panic(err)
		}
		if act.StopProcessing() {
			// abort SMTP transaction, you can use act.SMTPReply to send to the SMTP client
			panic(act.SMTPReply)
		}
		if act.Type == milter.ActionDiscard {
			// close the milter connection (do not send more SMTP events of this SMTP transaction)
			// but keep SMTP connection open and after DATA, silently discard the message
			panic(session.Close())
		}
	}

	// for each received SMTP command set relevant macros and send it to the milter
	macros.Set(milter.MacroIfAddr, "127.0.0.1")
	macros.Set(milter.MacroIfName, "eth0")
	handleMilterResponse(session.Conn("spammer.example.com", milter.FamilyInet, 0, "127.0.0.15"))

	macros.Set(milter.MacroSenderHostName, "spammer.example.com")
	macros.Set(milter.MacroTlsVersion, "SSLv3")
	handleMilterResponse(session.Helo("spammer.example.com"))

	macros.Set(milter.MacroMailMailer, "esmtp")
	macros.Set(milter.MacroMailHost, "example.com")
	macros.Set(milter.MacroMailAddr, "spammer@example.com")
	handleMilterResponse(session.Mail("<spammer@example.com>", ""))

	macros.Set(milter.MacroRcptMailer, "local")
	macros.Set(milter.MacroRcptHost, "example.com")
	macros.Set(milter.MacroRcptAddr, "other-spammer@example.com")
	handleMilterResponse(session.Rcpt("<other-spammer@example.com>", ""))

	macros.Set(milter.MacroRcptMailer, "local")
	macros.Set(milter.MacroRcptHost, "example.com")
	macros.Set(milter.MacroRcptAddr, "other-spammer2@example.com")
	handleMilterResponse(session.Rcpt("<other-spammer2@example.com>", ""))

	// After DataStart you should send the initial SMTP data to the first milter, accept and apply its modifications
	// and then send this modified data to the next milter. Before this point all milters could be queried in parallel.
	handleMilterResponse(session.DataStart())

	handleMilterResponse(session.HeaderField("From", "Your Bank <spammer@example.com>", nil))
	handleMilterResponse(session.HeaderField("To", "Your <spammer@example.com>", nil))
	handleMilterResponse(session.HeaderField("Subject", "Your money", nil))
	macros.SetHeaderDate(time.Date(2023, time.January, 1, 1, 1, 1, 0, time.UTC))
	handleMilterResponse(session.HeaderField("Date", "Sun, 1 Jan 2023 00:00:00 +0000", nil))

	handleMilterResponse(session.HeaderEnd())

	mActs, act, err := session.BodyReadFrom(strings.NewReader("Hello You,\r\ndo you want money?\r\nYour bank\r\n"))
	if err != nil {
		panic(err)
	}
	if act.StopProcessing() {
		panic(act.SMTPReply)
	}
	for _, mAct := range mActs {
		// process mAct
		log.Print(mAct)
	}
}
