package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/d--j/go-milter/integration"
	"github.com/emersion/go-smtp"
)

type Receiver struct {
	Msg          chan *integration.Output
	expectOutput bool
	m            sync.Mutex
	Config       *Config
	s            *smtp.Server
}

type receiverBackend struct {
	receiver *Receiver
}

type ReceiverSession struct {
	Hostname string
	Output   *integration.Output
	receiver *Receiver
}

func (rs *ReceiverSession) Reset() {
	rs.Output = nil
}

func (rs *ReceiverSession) Logout() error {
	return nil
}

func (rs *ReceiverSession) AuthPlain(_, _ string) error {
	return errors.New("no auth")
}

func (rs *ReceiverSession) Mail(from string, opts *smtp.MailOptions) error {
	if rs.Output == nil {
		rs.Output = &integration.Output{}
	}
	rs.Output.From = integration.ToAddrArg(from, opts)
	return nil
}

func (rs *ReceiverSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	rs.Output.To = append(rs.Output.To, integration.ToAddrArgRcpt(to, opts))
	return nil
}

func (rs *ReceiverSession) Data(r io.Reader) (err error) {
	var b []byte
	b, err = io.ReadAll(r)
	if err != nil {
		return
	}
	endHeaders := bytes.Index(b, []byte("\r\n\r\n"))
	if endHeaders < 0 {
		return fmt.Errorf("no end header marker found: %q", b)
	}
	rs.Output.Header = b[:endHeaders+4]
	rs.Output.Body = b[endHeaders+4:]
	rs.receiver.onMsg(rs.Output)
	return
}

func (r *receiverBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &ReceiverSession{Hostname: c.Hostname(), receiver: r.receiver}, nil
}

func (r *Receiver) Start() error {
	r.Msg = make(chan *integration.Output, 100)
	s := smtp.NewServer(&receiverBackend{receiver: r})
	s.Addr = fmt.Sprintf(":%d", r.Config.ReceiverPort)
	s.Domain = "localhost"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true
	s.EnableSMTPUTF8 = true
	s.EnableREQUIRETLS = true

	l, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}

	go func() {
		_ = s.Serve(l)
	}()

	r.s = s

	return nil
}

func (r *Receiver) clearMessages() {
	for {
		select {
		case o := <-r.Msg:
			r.onUnexpectedMsg(o)
		default:
			return
		}
	}
}

func (r *Receiver) ExpectMessage() {
	r.m.Lock()
	defer r.m.Unlock()
	r.expectOutput = true
	r.clearMessages()
}

func (r *Receiver) IgnoreMessages() {
	r.m.Lock()
	defer r.m.Unlock()
	r.expectOutput = false
	r.clearMessages()
}

var receiverMatch = regexp.MustCompile("(?ms)^Received:.*?(\r\n[^ \t])")

func (r *Receiver) WaitForMessage() *integration.Output {
	select {
	case <-time.After(time.Second * 20):
		return nil
	case o := <-r.Msg:
		if o.Header != nil {
			// replace the first received line with a placeholder
			loc := receiverMatch.FindIndex(o.Header)
			if len(loc) == 2 {
				o.Header = append(o.Header[:loc[0]], append([]byte("Received: placeholder"), o.Header[loc[1]-3:]...)...)
			}
		}
		return o
	}
}

func (r *Receiver) onMsg(output *integration.Output) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.expectOutput {
		r.Msg <- output
	} else {
		r.onUnexpectedMsg(output)
	}
}

func (r *Receiver) onUnexpectedMsg(output *integration.Output) {
	log.Printf("WARN: unexpected message received: %s", output)
}

func (r *Receiver) Cleanup() {
	_ = r.s.Close()
}
