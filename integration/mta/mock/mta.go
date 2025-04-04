package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/d--j/go-milter"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

type Rcpt struct {
	Addr, Args string
}

type Msg struct {
	From, FromArgs string
	Recipients     []Rcpt
	Data           []byte
}

var queue chan Msg

func errorFromResp(resp *milter.Action) *smtp.SMTPError {
	// github.com/emersion/go-smtp botches our multi-line error messages, so we escape it here and un-escape it the runner
	msg := resp.SMTPReply
	fixedMsg := "(!!!)" + strings.ReplaceAll(strings.ReplaceAll(msg, "\n", "\\n"), "\r", "\\r") + "(!!!)"
	return &smtp.SMTPError{
		Code:         int(resp.SMTPCode),
		EnhancedCode: smtp.NoEnhancedCode,
		Message:      fixedMsg,
	}
}

// The Backend implements SMTP server methods.
type Backend struct {
	client *milter.Client
}

func (bkd *Backend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	macros := milter.NewMacroBag()
	macros.Set(milter.MacroMTAVersion, "MOCK-SMTP 0.0.0")
	macros.Set(milter.MacroMTAFQDN, "localhost.local")
	macros.Set(milter.MacroDaemonName, "mock-smtp")
	macros.Set(milter.MacroIfName, "eth99")
	addr, _, err := net.SplitHostPort(conn.Conn().LocalAddr().String())
	if err != nil {
		return nil, err
	}
	macros.Set(milter.MacroIfAddr, addr)
	s, err := bkd.client.Session(macros)
	if err != nil {
		return nil, err
	}
	addr, portS, err := net.SplitHostPort(conn.Conn().RemoteAddr().String())
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portS, 10, 16)
	if err != nil {
		return nil, err
	}
	resp, err := s.Conn(addr, milter.FamilyInet, uint16(port), addr)
	if err != nil {
		return nil, err
	}
	if resp.StopProcessing() {
		return nil, errorFromResp(resp)
	}
	if state, ok := conn.TLSConnectionState(); ok {
		tlsVersion := map[uint16]string{
			tls.VersionTLS10: "TLSv1.0",
			tls.VersionTLS11: "TLSv1.1",
			tls.VersionTLS12: "TLSv1.2",
			tls.VersionTLS13: "TLSv1.3",
		}[state.Version]
		if tlsVersion == "" {
			tlsVersion = fmt.Sprintf("unknown(%x)", state.Version)
		}
		macros.Set(milter.MacroTlsVersion, tlsVersion)
		cipher := tls.CipherSuiteName(state.CipherSuite)
		bits := "256"
		if strings.Contains(cipher, "AES_128") {
			bits = "128"
		}
		macros.Set(milter.MacroCipher, cipher)
		macros.Set(milter.MacroCipherBits, bits)
	} else {
		macros.Set(milter.MacroTlsVersion, "")
		macros.Set(milter.MacroCipher, "")
		macros.Set(milter.MacroCipherBits, "")
	}
	resp, err = s.Helo(conn.Hostname())
	if err != nil {
		return nil, err
	}
	if resp.StopProcessing() {
		return nil, errorFromResp(resp)
	}
	queueId := randSeq(10)
	macros.Set(milter.MacroQueueId, queueId)
	return &Session{
		macros:  macros,
		filter:  s,
		queueId: queueId,
	}, nil
}

var _ smtp.Backend = (*Backend)(nil)

// A Session is returned after EHLO.
type Session struct {
	macros                 *milter.MacroBag
	filter                 *milter.ClientSession
	discarded              bool
	queueId                string
	MailFrom, MailFromArgs string
	Recipients             []Rcpt
	Header                 []byte
	Body                   []byte
	QuarantineReason       *string
}

func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *Session) Auth(_ string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		found := false
		if username == "user1@example.com" && password == "password1" {
			found = true
		}
		if username == "user2@example.com" && password == "password2" {
			found = true
		}
		if found {
			s.macros.Set(milter.MacroAuthType, "plain")
			s.macros.Set(milter.MacroAuthAuthen, username)
			log.Printf("[%s] Authenticated as: %s", s.queueId, username)
			return nil
		}
		return errors.New("invalid username or password")
	}), nil
}

func (s *Session) handleMilter(resp *milter.Action, err error) error {
	if err != nil {
		return err
	}
	if resp.StopProcessing() {
		return errorFromResp(resp)
	}
	if resp.Type == milter.ActionDiscard {
		s.discarded = true
	}
	return nil
}
func parseMailOptions(opts *smtp.MailOptions) string {
	var args []string
	if opts.Body != "" {
		args = append(args, fmt.Sprintf("BODY=%s", opts.Body))
	}
	if opts.Size > 0 {
		args = append(args, fmt.Sprintf("SIZE=%d", opts.Size))
	}
	if opts.UTF8 {
		args = append(args, "SMTPUTF8")
	}
	if opts.RequireTLS {
		args = append(args, "REQUIRETLS")
	}
	if opts.Auth != nil {
		args = append(args, fmt.Sprintf("AUTH=<%s>", *opts.Auth))
	}
	return strings.Join(args, " ")
}

func toMailOptions(arg string) *smtp.MailOptions {
	opts := smtp.MailOptions{}
	args := strings.Split(arg, " ")
	set := false
	for _, a := range args {
		if a == "REQUIRETLS" {
			opts.RequireTLS = true
			set = true
		} else if a == "SMTPUTF8" {
			opts.UTF8 = true
			set = true
		} else if strings.HasPrefix(a, "BODY=") {
			opts.Body = smtp.BodyType(a[5:])
			set = true
		} else if strings.HasPrefix(a, "SIZE=") {
			size, err := strconv.Atoi(a[5:])
			if err != nil {
				panic(err)
			}
			opts.Size = int64(size)
			set = true
		} else if strings.HasPrefix(a, "AUTH=") {
			auth := a[6 : len(a)-1]
			opts.Auth = &auth
			set = true
		}
	}
	if set {
		return &opts
	}
	return nil
}

func toRcptOptions(arg string) *smtp.RcptOptions {
	opts := smtp.RcptOptions{}
	args := strings.Split(arg, " ")
	set := false
	for _, a := range args {
		if strings.HasPrefix(a, "NOTIFY=") {
			foundNever := false
			for _, n := range strings.Split(a[7:len(a)-1], ",") {
				switch n {
				case string(smtp.DSNNotifyNever):
					opts.Notify = []smtp.DSNNotify{smtp.DSNNotifyNever}
					foundNever = true
					set = true
				case string(smtp.DSNNotifyDelayed), string(smtp.DSNNotifyFailure), string(smtp.DSNNotifySuccess):
					if !foundNever {
						opts.Notify = append(opts.Notify, smtp.DSNNotify(n))
						set = true
					}
				}
			}
		} else if strings.HasPrefix(a, "ORCPT=") {
			panic("we do not support this argument")
		}
	}
	if set {
		return &opts
	}
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.Printf("[%s] Mail from: %s", s.queueId, from)
	s.MailFrom = from
	s.MailFromArgs = parseMailOptions(opts)
	return s.handleMilter(s.filter.Mail(s.MailFrom, s.MailFromArgs))
}

func (s *Session) Rcpt(to string, _ *smtp.RcptOptions) error {
	log.Printf("[%s] Rcpt to: %s", s.queueId, to)
	if s.discarded {
		return nil
	}
	milterResp, err := s.filter.Rcpt(to, "")
	if err != nil {
		return err
	}
	if milterResp.Type == milter.ActionDiscard {
		s.discarded = true
	}
	if milterResp.StopProcessing() {
		return errorFromResp(milterResp)
	}
	s.Recipients = append(s.Recipients, Rcpt{Addr: to})
	return nil
}

func (s *Session) Data(r io.Reader) error {
	needsDiscard := true
	defer func() {
		if needsDiscard {
			_, _ = io.Copy(io.Discard, r)
		}
	}()
	if s.discarded {
		return nil
	}
	err := s.handleMilter(s.filter.DataStart())
	if err != nil {
		return err
	}
	if s.discarded {
		return nil
	}
	receivedHeader := strings.NewReader("Received: from mock ([127.0.0.1]) by mock with ESMTP for <someone@example.com>; Fri, 03 Mar 2023 22:11:17 +0100\r\n")
	data, err := io.ReadAll(io.MultiReader(receivedHeader, r))
	if err != nil {
		return err
	}
	index := bytes.Index(data, []byte("\r\n\r\n"))
	if index < 0 {
		return fmt.Errorf("could not find end of header in %q", data)
	}
	s.Header = data[:index+4]
	s.Body = data[index+4:]
	if len(s.Body) == 0 {
		return fmt.Errorf("empty body, header = %s", s.Header)
	}
	log.Printf("[%s] Data Lengths: Header: %d Body: %d", s.queueId, len(s.Header), len(s.Body))
	headers := splitHeaders(s.Header)
	for _, hdr := range headers {
		err = s.handleMilter(s.filter.HeaderField(hdr.key, string(hdr.raw[len(hdr.key)+1:]), nil))
		if err != nil {
			return err
		}
		if s.discarded {
			return nil
		}
	}
	err = s.handleMilter(s.filter.HeaderEnd())
	if err != nil {
		return err
	}
	if s.discarded {
		return nil
	}

	needsDiscard = false
	modActions, resp, err := s.filter.BodyReadFrom(bytes.NewReader(s.Body))
	err = s.handleMilter(resp, err)
	if err != nil {
		return err
	}
	if s.discarded {
		return nil
	}
	replacedBody := []byte(nil)

	for _, act := range modActions {
		switch act.Type {
		case milter.ActionChangeFrom:
			log.Printf("[%s] ACT = ActionChangeFrom %s %s", s.queueId, act.From, act.FromArgs)
			s.MailFrom = milter.RemoveAngle(act.From)
			s.MailFromArgs = act.FromArgs
		case milter.ActionDelRcpt:
			log.Printf("[%s] ACT = ActionDelRcpt %s", s.queueId, act.Rcpt)
			rcpt := milter.RemoveAngle(act.Rcpt)
		again:
			for i, r := range s.Recipients {
				if rcpt == r.Addr {
					s.Recipients = append(s.Recipients[:i], s.Recipients[i+1:]...)
					goto again
				}
			}
		case milter.ActionAddRcpt:
			log.Printf("[%s] ACT = ActionAddRcpt %s %s", s.queueId, act.Rcpt, act.RcptArgs)
			s.Recipients = append(s.Recipients, Rcpt{Addr: milter.RemoveAngle(act.Rcpt), Args: act.RcptArgs})
		case milter.ActionReplaceBody:
			log.Printf("[%s] ACT = ActionReplaceBody %q", s.queueId, act.Body)
			replacedBody = append(replacedBody, act.Body...)
		case milter.ActionQuarantine:
			log.Printf("[%s] ACT = ActionQuarantine %q", s.queueId, act.Reason)
			s.QuarantineReason = &act.Reason
		case milter.ActionAddHeader:
			log.Printf("[%s] ACT = ActionAddHeader %s %q", s.queueId, act.HeaderName, act.HeaderValue)
			maybeSpace := ""
			if len(act.HeaderValue) == 0 || (act.HeaderValue[0] != ' ' && act.HeaderValue[0] != '\t') {
				maybeSpace = " "
			}
			raw := fmt.Sprintf("%s:%s%s\r\n", act.HeaderName, maybeSpace, headerValue(act.HeaderValue))
			headers = append(headers, &field{
				key:       textproto.CanonicalMIMEHeaderKey(act.HeaderName),
				changeIdx: -1,
				raw:       []byte(raw),
			})
		case milter.ActionInsertHeader:
			log.Printf("[%s] ACT = ActionInsertHeader %d %s %q", s.queueId, act.HeaderIndex, act.HeaderName, act.HeaderValue)
			maybeSpace := ""
			if len(act.HeaderValue) == 0 || (act.HeaderValue[0] != ' ' && act.HeaderValue[0] != '\t') {
				maybeSpace = " "
			}
			raw := fmt.Sprintf("%s:%s%s\r\n", act.HeaderName, maybeSpace, headerValue(act.HeaderValue))
			f := &field{
				key:       textproto.CanonicalMIMEHeaderKey(act.HeaderName),
				changeIdx: -1,
				raw:       []byte(raw),
			}
			if act.HeaderIndex == 0 {
				headers = append([]*field{f}, headers...)
			} else if act.HeaderIndex == 1 { // special case: skip our received line
				headers = append(headers[:1], append([]*field{f}, headers[1:]...)...)
			} else if len(headers) < int(act.HeaderIndex)-1 {
				headers = append(headers, f)
			} else {
				idx := int(act.HeaderIndex) - 1
				headers = append(headers[:idx], append([]*field{f}, headers[idx:]...)...)
			}
		case milter.ActionChangeHeader:
			log.Printf("[%s] ACT = ActionChangeHeader %d %s %q", s.queueId, act.HeaderIndex, act.HeaderName, act.HeaderValue)
			maybeSpace := ""
			if len(act.HeaderValue) == 0 || (act.HeaderValue[0] != ' ' && act.HeaderValue[0] != '\t') {
				maybeSpace = " "
			}
			raw := fmt.Sprintf("%s:%s%s\r\n", act.HeaderName, maybeSpace, headerValue(act.HeaderValue))
			key := textproto.CanonicalMIMEHeaderKey(act.HeaderName)
			for _, f := range headers {
				if f.key == key && f.changeIdx == int(act.HeaderIndex) {
					if act.HeaderValue == "" {
						f.raw = nil
					} else {
						f.raw = []byte(raw)
					}
					break
				}
			}
		}
	}
	if s.QuarantineReason != nil {
		return nil
	}
	if replacedBody != nil {
		s.Body = replacedBody
	}

	data = nil
	for _, hdr := range headers {
		if hdr.raw != nil {
			data = append(data, hdr.raw...)
		}
	}
	if len(data) == 0 {
		data = append(data, '\r', '\n')
	}
	data = append(data, '\r', '\n')
	data = append(data, s.Body...)

	queue <- Msg{
		From:       s.MailFrom,
		FromArgs:   s.MailFromArgs,
		Recipients: s.Recipients,
		Data:       data,
	}

	return nil
}

func (s *Session) Reset() {
	log.Printf("[%s] Reset", s.queueId)
	_ = s.filter.Abort(nil)
	s.Body = nil
	s.Recipients = nil
}

func (s *Session) Logout() error {
	log.Printf("[%s] Logout", s.queueId)
	return nil
}

var _ smtp.Session = (*Session)(nil)

type field struct {
	key       string
	changeIdx int
	raw       []byte
}

func headerValue(s string) []byte {
	in := []byte(s)
	out := make([]byte, 0, len(in))
	l := len(in)
	for i := 0; i < l; i++ {
		out = append(out, in[i])
		if in[i] == '\n' && i+1 < l && (in[i+1] != ' ' && in[i+1] != '\t') {
			out = append(out, '\t')
		}
	}
	return out
}

func splitHeaders(headerBytes []byte) (fields []*field) {
	continuation := false
	keyCounter := make(map[string]int)
	var s []string
	for i := 0; i < len(headerBytes); {
		nextEnd := bytes.Index(headerBytes[i:], []byte("\r\n"))
		if nextEnd < 0 {
			panic("missing line ending")
		}
		nextEnd += 2
		s = append(s, string(headerBytes[i:i+nextEnd]))
		var peek byte
		if i+nextEnd < len(headerBytes) {
			peek = headerBytes[i+nextEnd]
		}
		continuation = peek == ' ' || peek == '\t'
		if !continuation {
			if s[0] == "\r\n" {
				// end marker
			} else {
				keyIdx := strings.IndexRune(s[0], ':')
				if keyIdx < 0 {
					log.Print(s)
					panic("key not found")
				}
				key := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(s[0][:keyIdx]))
				keyCounter[key] += 1
				fields = append(fields, &field{
					key:       key,
					changeIdx: keyCounter[key],
					raw:       []byte(strings.Join(s, "")),
				})
			}
			s = nil
		}
		i = i + nextEnd
	}
	return
}

//goland:noinspection SpellCheckingInspection
var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func sendQueue(nextHopAddr string) {
queue:
	for {
		msg := <-queue
		// Connect to the remote SMTP server.
		c, err := smtp.Dial(nextHopAddr)
		if err != nil {
			log.Print(err)
			continue
		}

		// Set the sender and recipient first
		if err := c.Mail(msg.From, toMailOptions(msg.FromArgs)); err != nil {
			log.Print(err)
			continue
		}
		for _, rcpt := range msg.Recipients {
			if err := c.Rcpt(rcpt.Addr, toRcptOptions(rcpt.Args)); err != nil {
				log.Print(err)
				continue queue
			}
		}

		// Send the email body.
		wc, err := c.Data()
		if err != nil {
			log.Print(err)
			continue
		}
		if _, err := wc.Write(msg.Data); err != nil {
			log.Print(err)
			_ = wc.Close()
			continue
		}
		if err = wc.Close(); err != nil {
			log.Print(err)
			continue
		}

		// Send the QUIT command and close the connection.
		if err = c.Quit(); err != nil {
			log.Print(err)
		}
	}
}

func main() {
	var mtaAddr string
	var milterAddr string
	var nextHopAddr string
	var tlsCert string
	var tlsKey string
	flag.StringVar(&mtaAddr, "mta", "", "mta address")
	flag.StringVar(&milterAddr, "milter", "", "milter address")
	flag.StringVar(&nextHopAddr, "next", "", "next hop address")
	flag.StringVar(&tlsCert, "cert", "", "path to TLS cert")
	flag.StringVar(&tlsKey, "key", "", "path to TLS key")
	flag.Parse()

	queue = make(chan Msg, 20)
	go sendQueue(nextHopAddr)

	s := smtp.NewServer(&Backend{client: milter.NewClient("tcp", milterAddr)})
	s.Addr = mtaAddr
	s.Domain = "localhost"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true
	s.EnableSMTPUTF8 = true
	s.EnableREQUIRETLS = true

	if tlsCert != "" && tlsKey != "" {
		cer, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			log.Fatal(err)
		}
		s.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cer},
		}
	}

	log.Println("Starting server at", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
