package milter

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	nettextproto "net/textproto"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/emersion/go-message/textproto"
)

type MockMilter struct {
	ConnResp *Response
	ConnMod  func(m *Modifier)
	ConnErr  error

	HeloResp *Response
	HeloMod  func(m *Modifier)
	HeloErr  error

	MailResp *Response
	MailMod  func(m *Modifier)
	MailErr  error

	RcptResp *Response
	RcptMod  func(m *Modifier)
	RcptErr  error

	DataResp *Response
	DataMod  func(m *Modifier)
	DataErr  error

	HdrResp *Response
	HdrMod  func(m *Modifier)
	HdrErr  error

	HdrsResp *Response
	HdrsMod  func(m *Modifier)
	HdrsErr  error

	BodyChunkResp *Response
	BodyChunkMod  func(m *Modifier)
	BodyChunkErr  error

	BodyResp *Response
	BodyMod  func(m *Modifier)
	BodyErr  error

	AbortMod func(m *Modifier)
	AbortErr error

	UnknownResp *Response
	UnknownMod  func(m *Modifier)
	UnknownErr  error

	OnClose func()

	// Info collected during calls.
	Host   string
	Family string
	Port   uint16
	Addr   string

	HeloValue string
	From      string
	FromEsmtp string
	Rcpt      []string
	RcptEsmtp []string
	Hdr       nettextproto.MIMEHeader

	Chunks [][]byte

	Cmds []string
}

func (mm *MockMilter) Connect(host string, family string, port uint16, addr string, m *Modifier) (*Response, error) {
	if mm.ConnMod != nil {
		mm.ConnMod(m)
	}
	mm.Host = host
	mm.Family = family
	mm.Port = port
	mm.Addr = addr
	return mm.ConnResp, mm.ConnErr
}

func (mm *MockMilter) Helo(name string, m *Modifier) (*Response, error) {
	if mm.HeloMod != nil {
		mm.HeloMod(m)
	}
	mm.HeloValue = name
	return mm.HeloResp, mm.HeloErr
}

func (mm *MockMilter) MailFrom(from string, esmtpArgs string, m *Modifier) (*Response, error) {
	if mm.MailMod != nil {
		mm.MailMod(m)
	}
	mm.From = from
	mm.FromEsmtp = esmtpArgs
	return mm.MailResp, mm.MailErr
}

func (mm *MockMilter) RcptTo(rcptTo string, esmtpArgs string, m *Modifier) (*Response, error) {
	if mm.RcptMod != nil {
		mm.RcptMod(m)
	}
	mm.Rcpt = append(mm.Rcpt, rcptTo)
	mm.RcptEsmtp = append(mm.RcptEsmtp, esmtpArgs)
	return mm.RcptResp, mm.RcptErr
}

func (mm *MockMilter) Data(m *Modifier) (*Response, error) {
	if mm.DataMod != nil {
		mm.DataMod(m)
	}
	return mm.DataResp, mm.DataErr
}

func (mm *MockMilter) Header(name string, value string, m *Modifier) (*Response, error) {
	if mm.HdrMod != nil {
		mm.HdrMod(m)
	}
	if mm.Hdr == nil {
		mm.Hdr = make(nettextproto.MIMEHeader)
	}
	mm.Hdr.Add(name, value)
	return mm.HdrResp, mm.HdrErr
}

func (mm *MockMilter) Headers(m *Modifier) (*Response, error) {
	if mm.HdrsMod != nil {
		mm.HdrsMod(m)
	}
	return mm.HdrsResp, mm.HdrsErr
}

func (mm *MockMilter) BodyChunk(chunk []byte, m *Modifier) (*Response, error) {
	if mm.BodyChunkMod != nil {
		mm.BodyChunkMod(m)
	}
	mm.Chunks = append(mm.Chunks, chunk)
	return mm.BodyChunkResp, mm.BodyChunkErr
}

func (mm *MockMilter) EndOfMessage(m *Modifier) (*Response, error) {
	if mm.BodyMod != nil {
		mm.BodyMod(m)
	}
	return mm.BodyResp, mm.BodyErr
}

func (mm *MockMilter) Abort(m *Modifier) error {
	if mm.AbortMod != nil {
		mm.AbortMod(m)
	}
	return mm.AbortErr
}

func (mm *MockMilter) Unknown(cmd string, m *Modifier) (*Response, error) {
	if mm.UnknownMod != nil {
		mm.UnknownMod(m)
	}
	mm.Cmds = append(mm.Cmds, cmd)
	return mm.UnknownResp, mm.UnknownErr
}

func (mm *MockMilter) Cleanup() {
	if mm.OnClose != nil {
		mm.OnClose()
	}
}

func assertAction(t *testing.T, act *Action, err error, expectCode ActionType) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if act.Type != expectCode {
		t.Fatalf("Unexpected code %c: %+v", act.Type, act)
	}
}

type serverClientWrap struct {
	server  *Server
	client  *Client
	session *ClientSession
	local   net.Listener
}

func newServerClient(t *testing.T, macros Macros, serverOptions []Option, clientOptions []Option) serverClientWrap {
	var err error
	s := NewServer(serverOptions...)
	w := serverClientWrap{server: s}
	w.local, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		s.Serve(w.local)
	}()
	w.client = NewClient("tcp", w.local.Addr().String(), clientOptions...)
	w.session, err = w.client.Session(macros)
	if err != nil {
		w.server.Close()
		t.Fatal(err)
	}
	return w
}

func (w *serverClientWrap) Cleanup() {
	w.session.Close()
	w.server.Close()
}

func TestMilterClient_UsualFlow(t *testing.T) {
	t.Parallel()
	mm := MockMilter{
		ConnResp: RespContinue,
		HeloResp: RespContinue,
		MailResp: RespContinue,
		RcptResp: RespContinue,
		DataResp: RespContinue,
		HdrResp:  RespContinue,
		HdrsResp: RespContinue,
		HdrsMod: func(m *Modifier) {
			m.Progress()
		},
		BodyChunkResp: RespContinue,
		BodyResp:      RespContinue,
		BodyMod: func(m *Modifier) {
			m.ChangeFrom("changed@example.com", "")
			m.ChangeFrom("changed@example.com", "A=B")
			m.AddRecipient("example@example.com", "")
			m.AddRecipient("<example@example.com>", "A=B")
			m.DeleteRecipient("del@example.com")
			m.AddHeader("X-Bad", "very")
			m.Progress()
			m.ChangeHeader(1, "Subject", "***SPAM***")
			m.InsertHeader(2, "X-Hdr", "value")
			m.Quarantine("very bad message")
			m.ReplaceBody(strings.NewReader(strings.Repeat("-", int(DataSize64K)+1)))
		},
		UnknownResp: RespContinue,
	}
	macros := NewMacroBag()
	w := newServerClient(t, macros, []Option{WithMilter(func() Milter {
		return &mm
	}), WithActions(OptAddHeader | OptChangeBody | OptAddRcpt | OptRemoveRcpt | OptChangeHeader | OptQuarantine | OptChangeFrom | OptAddRcptWithArgs)},
		[]Option{WithActions(OptAddHeader | OptChangeBody | OptAddRcpt | OptRemoveRcpt | OptChangeHeader | OptQuarantine | OptChangeFrom | OptAddRcptWithArgs)},
	)
	defer w.Cleanup()

	macros.Set(MacroTlsVersion, "very old")
	act, err := w.session.Conn("host", FamilyInet, 25565, "172.0.0.1")
	assertAction(t, act, err, ActionContinue)
	if mm.Host != "host" {
		t.Fatal("Wrong host:", mm.Host)
	}
	if mm.Family != "tcp4" {
		t.Fatal("Wrong family:", mm.Family)
	}
	if mm.Port != 25565 {
		t.Fatal("Wrong port:", mm.Port)
	}
	if mm.Addr != "172.0.0.1" {
		t.Fatal("Wrong IP:", mm.Addr)
	}

	act, err = w.session.Helo("helo_host")
	assertAction(t, act, err, ActionContinue)
	if mm.HeloValue != "helo_host" {
		t.Fatal("Wrong helo value:", mm.HeloValue)
	}

	act, err = w.session.Mail("from@example.org", "A=B")
	assertAction(t, act, err, ActionContinue)
	if mm.From != "from@example.org" {
		t.Fatal("Wrong MAIL FROM:", mm.From)
	}

	act, err = w.session.Rcpt("to1@example.org", "A=B")
	assertAction(t, act, err, ActionContinue)
	act, err = w.session.Rcpt("to2@example.org", "A=C")
	assertAction(t, act, err, ActionContinue)
	if !reflect.DeepEqual(mm.Rcpt, []string{"to1@example.org", "to2@example.org"}) {
		t.Fatal("Wrong recipients:", mm.Rcpt)
	}
	if !reflect.DeepEqual(mm.RcptEsmtp, []string{"A=B", "A=C"}) {
		t.Fatal("Wrong recipients esmtp args:", mm.RcptEsmtp)
	}

	hdr := textproto.Header{}
	hdr.Add("From", "from@example.org")
	hdr.Add("To", "to@example.org")
	hdr.Add("x-empty-header", "")
	act, err = w.session.Header(hdr) // calls DataStart() automatically
	assertAction(t, act, err, ActionContinue)
	if len(mm.Hdr) != 3 {
		t.Fatal("Unexpected header length:", len(mm.Hdr))
	}
	if val := mm.Hdr.Get("From"); val != "from@example.org" {
		t.Fatal("Wrong From header:", val)
	}
	if val := mm.Hdr.Get("To"); val != "to@example.org" {
		t.Fatal("Wrong To header:", val)
	}
	if val := mm.Hdr.Get("x-empty-header"); val != "" {
		t.Fatal("Wrong To header:", val)
	}

	act, err = w.session.Unknown("INVALID command", map[MacroName]string{MacroHopCount: "2"})
	assertAction(t, act, err, ActionContinue)
	if !reflect.DeepEqual(mm.Cmds, []string{"INVALID command"}) {
		t.Fatal("Wrong cmds:", mm.Cmds)
	}

	modifyActs, act, err := w.session.BodyReadFrom(bytes.NewReader(bytes.Repeat([]byte{'A'}, 128000)))
	assertAction(t, act, err, ActionContinue)

	if len(mm.Chunks) != 2 {
		t.Fatal("Wrong amount of body chunks received")
	}
	if len(mm.Chunks[0]) > 65535 {
		t.Fatal("Too big first chunk:", len(mm.Chunks[0]))
	}
	if totalLen := len(mm.Chunks[0]) + len(mm.Chunks[1]); totalLen < 128000 {
		t.Fatal("Some body bytes lost:", totalLen)
	}

	firstBodyChunk := []byte(strings.Repeat("-", int(DataSize64K)))
	expected := []ModifyAction{
		{Type: ActionChangeFrom, From: "<changed@example.com>"},
		{Type: ActionChangeFrom, From: "<changed@example.com>", FromArgs: "A=B"},
		{Type: ActionAddRcpt, Rcpt: "<example@example.com>"},
		{Type: ActionAddRcpt, Rcpt: "<example@example.com>", RcptArgs: "A=B"},
		{Type: ActionDelRcpt, Rcpt: "<del@example.com>"},
		{Type: ActionAddHeader, HeaderName: "X-Bad", HeaderValue: "very"},
		{Type: ActionChangeHeader, HeaderIndex: 1, HeaderName: "Subject", HeaderValue: "***SPAM***"},
		{Type: ActionInsertHeader, HeaderIndex: 2, HeaderName: "X-Hdr", HeaderValue: "value"},
		{Type: ActionQuarantine, Reason: "very bad message"},
		{Type: ActionReplaceBody, Body: firstBodyChunk},
		{Type: ActionReplaceBody, Body: []byte{'-'}},
	}
	if !reflect.DeepEqual(modifyActs, expected) {
		t.Fatalf("Wrong modify actions: got %+v", modifyActs)
	}
}

func TestMilterClient_AbortFlow(t *testing.T) {
	t.Parallel()
	waitChan := make(chan interface{}, 2)
	heloTls := "not set"
	aborTls := "not set"
	mailAuthen := "not set"
	mm := MockMilter{
		ConnResp: RespContinue,
		HeloResp: RespContinue,
		HeloMod: func(m *Modifier) {
			heloTls = m.Macros.Get(MacroTlsVersion)
		},
		MailResp: RespContinue,
		MailMod: func(m *Modifier) {
			mailAuthen = m.Macros.Get(MacroAuthAuthen)
		},
		AbortMod: func(m *Modifier) {
			aborTls = m.Macros.Get(MacroTlsVersion)
			waitChan <- nil
		},
	}
	macros := NewMacroBag()
	w := newServerClient(t, macros, []Option{WithMilter(func() Milter {
		return &mm
	}), WithActions(OptAddHeader | OptChangeHeader)}, []Option{WithActions(OptAddHeader | OptChangeHeader | OptQuarantine)})
	defer w.Cleanup()

	act, err := w.session.Conn("host", FamilyInet, 25565, "172.0.0.1")
	assertAction(t, act, err, ActionContinue)
	if mm.Host != "host" {
		t.Fatal("Wrong host:", mm.Host)
	}
	if mm.Family != "tcp4" {
		t.Fatal("Wrong family:", mm.Family)
	}
	if mm.Port != 25565 {
		t.Fatal("Wrong port:", mm.Port)
	}
	if mm.Addr != "172.0.0.1" {
		t.Fatal("Wrong IP:", mm.Addr)
	}

	macros.Set(MacroTlsVersion, "very old")
	act, err = w.session.Helo("helo_host")
	assertAction(t, act, err, ActionContinue)
	if mm.HeloValue != "helo_host" {
		t.Fatal("Wrong helo value:", mm.HeloValue)
	}

	if heloTls != "very old" {
		t.Fatal("Wrong tls_version macro value:", heloTls)
	}

	macros.Set(MacroAuthAuthen, "login-user")
	act, err = w.session.Mail("login-user@example.com", "")
	assertAction(t, act, err, ActionContinue)
	if mm.From != "login-user@example.com" {
		t.Fatal("Wrong from value:", mm.From)
	}
	if mailAuthen != "login-user" {
		t.Fatal("Unexpected macro data:", mailAuthen)
	}

	err = w.session.Abort(nil)
	<-waitChan // since Abort() does not wait for a response we need to wait for the server to finish on our own
	if err != nil {
		t.Fatal(err)
	}

	// Validate macro values are preserved for the abort callback
	if aborTls != "very old" {
		t.Fatal("Wrong tls_version macro value: ", aborTls)
	}

	macros.Set(MacroAuthAuthen, "")
	act, err = w.session.Mail("another-user@example.com", "")
	assertAction(t, act, err, ActionContinue)
	if mm.From != "another-user@example.com" {
		t.Fatal("Wrong from value:", mm.From)
	}
	if len(mailAuthen) != 0 {
		t.Fatal("Unexpected macro data:", mailAuthen)
	}
}

func TestMilterClient_NoWorking(t *testing.T) {
	t.Parallel()
	mm := MockMilter{
		MailResp: RespReject,
	}
	w := newServerClient(t, nil, []Option{WithMilter(func() Milter {
		return &mm
	}), WithActions(OptAddHeader | OptChangeHeader), WithProtocols(OptNoMailFrom)},
		[]Option{WithActions(OptAddHeader | OptChangeHeader | OptQuarantine)},
	)
	defer w.Cleanup()

	_, err := w.session.Mail("from@example.org", "A=B")
	if err == nil || err.Error() != "milter: in wrong state 1" {
		t.Fatal("expected error")
	}
	w.local.Close()

	cl2 := NewClient(w.local.Addr().Network(), w.local.Addr().String())
	if _, err := cl2.Session(nil); err == nil {
		t.Fatal("could start a session to a non-existing server")
	}
}

func TestMilterClient_NegotiationMismatch(t *testing.T) {
	t.Parallel()
	mm := MockMilter{}
	s := NewServer(WithMilter(func() Milter {
		return &mm
	}), WithActions(OptAddHeader|OptChangeHeader), WithProtocols(OptNoMailFrom))
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve(local)
	client := NewClient("tcp", local.Addr().String(), WithActions(OptAddHeader|OptChangeHeader|OptQuarantine), WithProtocols(OptNoEOH))
	session, err := client.Session(nil)
	if err == nil {
		session.Close()
		t.Fatal("negotiation should fail")
	}

	client2 := NewClient("tcp", local.Addr().String(), WithActions(OptAddHeader), WithProtocols(OptNoMailFrom))
	session2, err := client2.Session(nil)
	if err == nil {
		session2.Close()
		t.Fatal("negotiation should fail")
	}
}

func TestMilterClient_BogusServerNegotiation(t *testing.T) {
	tests := []struct {
		name        string
		opts        []Option
		negResponse []byte
		onlyWarning bool
	}{
		{"not even full packet", []Option{WithReadTimeout(time.Second)}, []byte{0}, false},
		{"wrong response code", nil, []byte{0, 0, 0, 1, 'a'}, false},
		{"too few bytes", nil, []byte{0, 0, 0, 2, byte(wire.CodeOptNeg), 0}, false},
		{"milter version 0", nil, []byte{0, 0, 0, 13, byte(wire.CodeOptNeg), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{"milter version 1", nil, []byte{0, 0, 0, 13, byte(wire.CodeOptNeg), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{"wrong actions", nil, []byte{0, 0, 0, 13, byte(wire.CodeOptNeg), 0, 0, 0, 2, 255, 255, 255, 255, 0, 0, 0, 0}, false},
		{"wrong protocol", nil, []byte{0, 0, 0, 13, byte(wire.CodeOptNeg), 0, 0, 0, 2, 0, 0, 0, 0, 255, 255, 255, 255}, false},
		{"wrong milter stage", nil, []byte{0, 0, 0, 18, byte(wire.CodeOptNeg), 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255, 255, 0}, true},
		{"wrong milter list", nil, []byte{0, 0, 0, 18, byte(wire.CodeOptNeg), 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'a'}, true},
		{"repeated milter stage", nil, []byte{0, 0, 0, 25, byte(wire.CodeOptNeg), 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'a', 0, 0, 0, 0, 0, 'a', 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			// t.Parallel() - test cannot be Parallel() because it replaces the global LogWarning
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			// just slurp up everything the client sends
			go func() {
				buf := make([]byte, 1024)
				for {
					_ = serverConn.SetReadDeadline(time.Now().Add(time.Minute))
					if _, err := serverConn.Read(buf); err != nil {
						if err != io.EOF && err != io.ErrClosedPipe {
							t.Logf("server got error: read: %v", err)
						}
						return
					}
				}
			}()
			sErrChan := make(chan error)
			go func() {
				if _, err := serverConn.Write(ltt.negResponse); err != nil {
					sErrChan <- err
					return
				}
				sErrChan <- nil
			}()
			warningCalled := false
			if ltt.onlyWarning {
				LogWarning = func(format string, v ...interface{}) {
					warningCalled = true
					logWarning(format, v...)
				}
			}
			cl := NewClient(clientConn.LocalAddr().Network(), clientConn.LocalAddr().String(), ltt.opts...)
			session, err := cl.session(clientConn, nil)
			if ltt.onlyWarning {
				LogWarning = logWarning
				if session == nil {
					t.Fatalf("negotiation should succeed but it did not with server response %x", ltt.negResponse)
				}
				session.Close()
				if !warningCalled {
					t.Fatal("negotiation should have called a warning")
				}
			} else {
				if err == nil {
					session.Close()
					t.Fatalf("expected error in negotiation but it succeeded with server response %x", ltt.negResponse)
				}
			}

			if err := <-sErrChan; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMilterClient_Negotiation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		opts           []Option
		serverVersion  uint32
		serverActions  OptAction
		serverProtocol OptProtocol
		wantVersion    uint32
		wantActions    OptAction
		wantProtocol   OptProtocol
		wantBufferSize DataSize
	}{
		{"default", nil, MaxClientProtocolVersion, OptAddHeader, 0, MaxClientProtocolVersion, OptAddHeader, 0, DataSize64K},
		{"v6 client v2 server", nil, 2, OptAddHeader, 0, 2, OptAddHeader, OptNoUnknown | OptNoData, DataSize64K},
		{"v2 client v2 server", []Option{WithMaximumVersion(2), WithProtocols(allClientSupportedProtocolMasksV2), WithActions(allClientSupportedActionMasksV2)}, 2, OptAddHeader, 0, 2, OptAddHeader, OptNoUnknown | OptNoData, DataSize64K},
		{"offered 1MB not accepted", []Option{WithActions(AllClientSupportedActionMasks), WithOfferedMaxData(DataSize1M)}, MaxClientProtocolVersion, OptAddHeader, 0, MaxClientProtocolVersion, OptAddHeader, 0, DataSize64K},
		{"offered 256K not accepted", []Option{WithActions(AllClientSupportedActionMasks), WithOfferedMaxData(DataSize256K)}, MaxClientProtocolVersion, OptAddHeader, 0, MaxClientProtocolVersion, OptAddHeader, 0, DataSize64K},
		{"offered 1MB accepted", []Option{WithActions(AllClientSupportedActionMasks), WithOfferedMaxData(DataSize1M)}, MaxClientProtocolVersion, OptAddHeader, OptProtocol(optMds1M), MaxClientProtocolVersion, OptAddHeader, 0, DataSize1M},
		{"offered 256K accepted", []Option{WithActions(AllClientSupportedActionMasks), WithOfferedMaxData(DataSize256K)}, MaxClientProtocolVersion, OptAddHeader, OptProtocol(optMds256K), MaxClientProtocolVersion, OptAddHeader, 0, DataSize256K},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()

			// just slurp up everything the client sends
			go func() {
				buf := make([]byte, 1024)
				for {
					_ = serverConn.SetReadDeadline(time.Now().Add(time.Minute))
					if _, err := serverConn.Read(buf); err != nil {
						if err != io.EOF && err != io.ErrClosedPipe {
							t.Logf("server got error: read: %v", err)
						}
						return
					}
				}
			}()
			sErrChan := make(chan error)
			go func() {
				defer serverConn.Close()
				response := []byte{0, 0, 0, 13, byte(wire.CodeOptNeg), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
				binary.BigEndian.PutUint32(response[5:], ltt.serverVersion)
				binary.BigEndian.PutUint32(response[9:], uint32(ltt.serverActions))
				binary.BigEndian.PutUint32(response[13:], uint32(ltt.serverProtocol))
				if _, err := serverConn.Write(response); err != nil {
					sErrChan <- err
					return
				}
				sErrChan <- nil
			}()
			cl := NewClient(clientConn.LocalAddr().Network(), clientConn.LocalAddr().String(), ltt.opts...)
			session, err := cl.session(clientConn, nil)
			if err != nil {
				t.Fatalf("expected no error in negotiation but got %v, with server version %d actions %x protocol %x", err, ltt.serverVersion, ltt.serverActions, ltt.serverProtocol)
			}
			if session.version != ltt.wantVersion {
				t.Fatalf("version: got %d expected %d", session.version, ltt.wantVersion)
			}
			if session.actionOpts != ltt.wantActions {
				t.Fatalf("actions: got %032b expected %032b", session.actionOpts, ltt.wantActions)
			}
			if session.protocolOpts != ltt.wantProtocol {
				t.Fatalf("protocol: got %032b expected %032b", session.protocolOpts, ltt.wantProtocol)
			}
			if session.negotiatedBodySize != uint32(ltt.wantBufferSize) {
				t.Fatalf("buffer size: got %d expected %d", session.negotiatedBodySize, ltt.wantBufferSize)
			}
			session.Close()

			if err := <-sErrChan; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMilterClient_WithMockServer(t *testing.T) {
	t.Parallel()
	type op struct {
		s1     func(*ClientSession) (*Action, error)
		s2     func(*ClientSession) error
		s3     func(*ClientSession) ([]ModifyAction, *Action, error)
		v1     func(*testing.T, *ClientSession, *Action, error)
		v2     func(*testing.T, *ClientSession, error)
		v3     func(*testing.T, *ClientSession, []ModifyAction, *Action, error)
		server []byte
	}
	type ops []op

	type cfg struct {
		Opts              []Option
		ServerNegotiation []byte
		Macros            Macros
	}

	withProtC := func(prot OptProtocol) cfg {
		c := cfg{
			Opts:              []Option{WithActions(AllClientSupportedActionMasks), WithReadTimeout(time.Second), WithWriteTimeout(time.Second)},
			ServerNegotiation: []byte{0, 0, 0, 13, byte(wire.CodeOptNeg), 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 0},
			Macros:            nil,
		}
		binary.BigEndian.PutUint32(c.ServerNegotiation[13:], uint32(prot))
		return c
	}
	withActC := func(c cfg, act OptAction) cfg {
		binary.BigEndian.PutUint32(c.ServerNegotiation[9:], uint32(act))
		return c
	}
	with256KbC := func(c cfg) cfg {
		c.Opts = append(c.Opts, WithOfferedMaxData(DataSize256K))
		binary.BigEndian.PutUint32(c.ServerNegotiation[13:], optMds256K|binary.BigEndian.Uint32(c.ServerNegotiation[13:]))
		return c
	}
	dC := withProtC(0)

	sendConnect := func(s *ClientSession) (*Action, error) {
		return s.Conn("localhost", FamilyUnix, 0, "/var/run/sock")
	}
	sendHelo := func(s *ClientSession) (*Action, error) {
		return s.Helo("localhost")
	}
	sendMail := func(s *ClientSession) (*Action, error) {
		return s.Mail("", "")
	}
	sendRcpt := func(s *ClientSession) (*Action, error) {
		return s.Rcpt("", "")
	}
	sendData := func(s *ClientSession) (*Action, error) {
		return s.DataStart()
	}
	sendHeaderField := func(s *ClientSession) (*Action, error) {
		return s.HeaderField("a", "b", nil)
	}
	sendHeaderEnd := func(s *ClientSession) (*Action, error) {
		return s.HeaderEnd()
	}
	sendBodyChunk := func(s *ClientSession) (*Action, error) {
		return s.BodyChunk([]byte("line\n"))
	}
	sendEnd := func(s *ClientSession) ([]ModifyAction, *Action, error) {
		return s.End()
	}

	expectErr1 := func(t *testing.T, _ *ClientSession, act *Action, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected err but got act = %+v", act)
		}
	}
	expectAct := func(expectedActCode ActionType, t *testing.T, act *Action, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if act.Type != expectedActCode {
			t.Fatalf("expected %c, got %+v", expectedActCode, act)
		}
	}
	expectContinue := func(t *testing.T, _ *ClientSession, act *Action, err error) {
		t.Helper()
		expectAct(ActionContinue, t, act, err)
	}
	expectReject := func(t *testing.T, _ *ClientSession, act *Action, err error) {
		t.Helper()
		expectAct(ActionReject, t, act, err)
	}
	expectAcceptEmptyMods := func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
		t.Helper()
		expectAct(ActionAccept, t, act, err)
		if len(mActs) > 0 {
			t.Fatalf("expected empty modifications, got %+v", mActs)
		}
	}

	responseContinue := []byte{0, 0, 0, 1, byte(wire.ActContinue)}

	tests := []struct {
		name string
		cfg  cfg
		ops  ops
	}{
		{"bogus response at connect", dC, ops{{s1: sendConnect, v1: expectErr1, server: []byte{0, 0, 0}}}},
		{"double connect", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendConnect, v1: expectErr1, server: responseContinue},
		}},
		{"Progress response working", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: []byte{0, 0, 0, 1, byte(wire.ActProgress), 0, 0, 0, 1, byte(wire.ActProgress), 0, 0, 0, 1, byte(wire.ActContinue)}},
		}},
		{"ActReplyCode response working", dC, ops{
			{s1: sendConnect, v1: func(t *testing.T, s *ClientSession, act *Action, err error) {
				expectAct(ActionRejectWithCode, t, act, err)
				if act.SMTPCode != 400 {
					t.Fatalf("expected code %d, got %d", 400, act.SMTPCode)
				}
				if act.SMTPReply != "400 T" {
					t.Fatalf("expected text %s, got %s", "400 T", act.SMTPReply)
				}
			}, server: []byte{0, 0, 0, 7, byte(wire.ActReplyCode), '4', '0', '0', ' ', 'T', 0}},
		}},
		{"ActReplyCode parsing error 1", dC, ops{
			{s1: sendConnect, v1: expectErr1, server: []byte{0, 0, 0, 7, byte(wire.ActReplyCode), 'a', '0', '0', ' ', 'T', 0}},
		}},
		{"ActReplyCode parsing error 2", dC, ops{
			{s1: sendConnect, v1: expectErr1, server: []byte{0, 0, 0, 4, byte(wire.ActReplyCode), '4', '0', '0'}},
		}},
		{"OptNoConnect working", withProtC(OptNoConnect), ops{
			{s1: sendConnect, v1: expectContinue, server: nil},
		}},
		{"OptNoConnReply working", withProtC(OptNoConnReply), ops{
			{s1: sendConnect, v1: expectContinue, server: nil},
		}},
		{"premature Helo", dC,
			ops{{s1: sendHelo, v1: expectErr1, server: responseContinue}},
		},
		{"bogus response at helo", dC,
			ops{{s1: sendConnect, v1: expectContinue, server: responseContinue}, {s1: sendHelo, v1: expectErr1, server: []byte{0, 0, 0}}},
		},
		{"OptNoHelo working", withProtC(OptNoHelo), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: nil},
		}},
		{"OptNoHeloReply working", withProtC(OptNoHeloReply), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: nil},
		}},
		{"premature Mail", withProtC(OptNoMailFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectErr1, server: responseContinue},
		}},
		{"OptNoMailFrom working", withProtC(OptNoMailFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: nil},
		}},
		{"OptNoMailReply working", withProtC(OptNoMailReply), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: nil},
		}},
		{"premature Rcpt", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectErr1, server: responseContinue},
		}},
		{"OptNoRcptTo working", withProtC(OptNoRcptTo), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: nil},
		}},
		{"OptNoRcptReply working", withProtC(OptNoRcptReply), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: nil},
		}},
		{"premature DataStart", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectErr1, server: responseContinue},
		}},
		{"OptNoData working", withProtC(OptNoData), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: nil},
		}},
		{"OptNoDataReply working", withProtC(OptNoDataReply), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: nil},
		}},
		{"premature HeaderField", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectErr1, server: responseContinue},
		}},
		{"OptNoHeaders working", withProtC(OptNoHeaders), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: nil},
		}},
		{"OptNoHeaderReply working", withProtC(OptNoHeaderReply), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: nil},
		}},
		{"premature HeaderEnd", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectErr1, server: responseContinue},
		}},
		{"OptNoEOH working", withProtC(OptNoEOH), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: nil},
		}},
		{"OptNoEOHReply working", withProtC(OptNoEOHReply), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: nil},
		}},
		{"premature BodyChunk", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectErr1, server: responseContinue},
		}},
		{"Skip working", withProtC(OptSkip), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: []byte{0, 0, 0, 1, byte(wire.ActSkip)}},
			{s1: sendBodyChunk, v1: expectContinue, server: nil},
		}},
		{"Skip rejected 1", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectErr1, server: []byte{0, 0, 0, 1, byte(wire.ActSkip)}},
		}},
		{"Skip rejected 2", dC, ops{
			{s1: sendConnect, v1: expectErr1, server: []byte{0, 0, 0, 1, byte(wire.ActSkip)}},
		}},
		{"Reject too much data", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				return s.BodyChunk(make([]byte, DataSize256K))
			}, v1: expectErr1, server: responseContinue},
		}},
		{"BodyChunk Skip working", withProtC(OptSkip), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				act, err := s.BodyChunk([]byte("line\n"))
				if err != nil || act.Type != ActionContinue {
					if err == nil {
						err = fmt.Errorf("expected continue response, got %+v", act)
					}
					return nil, err
				}
				if !s.Skip() {
					return nil, fmt.Errorf("expected Skip to be true")
				}
				act, err = s.BodyChunk([]byte("line\n"))
				if err != nil || act.Type != ActionContinue {
					if err == nil {
						err = fmt.Errorf("expected continue response, got %+v", act)
					}
					return nil, err
				}
				return act, err
			}, v1: expectContinue, server: []byte{0, 0, 0, 1, byte(wire.ActSkip)}},
		}},
		{"OptNoBody working", withProtC(OptNoBody), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: nil},
		}},
		{"OptNoBodyReply working", withProtC(OptNoBodyReply), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: nil},
		}},
		{"Header working", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				hdrs := textproto.Header{}
				hdrs.Add("From", "<>")
				hdrs.Add("To", "<>")
				return s.Header(hdrs)
			}, v1: expectContinue, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue)}},
		}},
		{"Header after HeaderField", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				hdrs := textproto.Header{}
				hdrs.Add("From", "<>")
				hdrs.Add("To", "<>")
				return s.Header(hdrs)
			}, v1: expectContinue, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue)}},
		}},
		{"Header auto Data", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				hdrs := textproto.Header{}
				hdrs.Add("From", "<>")
				hdrs.Add("To", "<>")
				return s.Header(hdrs)
			}, v1: expectContinue, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue)}},
		}},
		{"Header auto Data reject", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				hdrs := textproto.Header{}
				hdrs.Add("From", "<>")
				hdrs.Add("To", "<>")
				return s.Header(hdrs)
			}, v1: expectReject, server: []byte{0, 0, 0, 1, byte(wire.ActReject)}},
		}},
		{"Header premature", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				hdrs := textproto.Header{}
				hdrs.Add("From", "<>")
				hdrs.Add("To", "<>")
				return s.Header(hdrs)
			}, v1: expectErr1, server: responseContinue},
		}},
		{"Header Reject second header", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: func(s *ClientSession) (*Action, error) {
				hdrs := textproto.Header{}
				hdrs.Add("From", "<>")
				hdrs.Add("To", "<>")
				return s.Header(hdrs)
			}, v1: expectReject, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActReject)}},
		}},
		{"BodyReadFrom working", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s3: func(s *ClientSession) ([]ModifyAction, *Action, error) {
				return s.BodyReadFrom(bytes.NewReader(make([]byte, 3*DataSize64K)))
			}, v3: expectAcceptEmptyMods, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"BodyReadFrom skip working", withProtC(OptSkip), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s3: func(s *ClientSession) ([]ModifyAction, *Action, error) {
				return s.BodyReadFrom(bytes.NewReader(make([]byte, 3*DataSize64K)))
			}, v3: expectAcceptEmptyMods, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActSkip), 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"BodyReadFrom accept mid-stream", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s3: func(s *ClientSession) ([]ModifyAction, *Action, error) {
				return s.BodyReadFrom(bytes.NewReader(make([]byte, 3*DataSize64K)))
			}, v3: expectAcceptEmptyMods, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"BodyReadFrom premature", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s3: func(s *ClientSession) ([]ModifyAction, *Action, error) {
				return s.BodyReadFrom(bytes.NewReader(make([]byte, 3*DataSize64K)))
			}, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 1, byte(wire.ActContinue), 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActAddRcpt working", withActC(withProtC(0), OptAddRcpt), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionAddRcpt, Rcpt: "<>"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 4, byte(wire.ActAddRcpt), '<', '>', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"End with ActProgress working", withActC(withProtC(0), OptAddRcpt), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionAddRcpt, Rcpt: "<>"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 1, byte(wire.ActProgress), 0, 0, 0, 1, byte(wire.ActProgress), 0, 0, 0, 4, byte(wire.ActAddRcpt), '<', '>', 0, 0, 0, 0, 1, byte(wire.ActProgress), 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"End premature", dC, ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActAddRcpt error detection 1", withActC(withProtC(0), OptAddRcpt), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 5, byte(wire.ActAddRcpt), '<', '>', 0, 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActAddRcpt error detection 2", withActC(withProtC(0), OptAddRcpt), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 1, byte(wire.ActAddRcpt), 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"OptAddRcptWithArgs working", withActC(withProtC(0), OptAddRcptWithArgs), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionAddRcpt, Rcpt: "<>", RcptArgs: "A"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 6, byte(wire.ActAddRcptPar), '<', '>', 0, 'A', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"OptAddRcptWithArgs error detection 1", withActC(withProtC(0), OptAddRcptWithArgs), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 8, byte(wire.ActAddRcpt), '<', '>', 0, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"OptAddRcptWithArgs error detection 2", withActC(withProtC(0), OptAddRcptWithArgs), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 1, byte(wire.ActAddRcpt), 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActDelRcpt working", withActC(withProtC(0), OptRemoveRcpt), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionDelRcpt, Rcpt: "<>"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 4, byte(wire.ActDelRcpt), '<', '>', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActQuarantine working", withActC(withProtC(0), OptQuarantine), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionQuarantine, Reason: "test"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 6, byte(wire.ActQuarantine), 't', 'e', 's', 't', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActReplBody working", withActC(withProtC(0), OptChangeBody), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionReplaceBody, Body: []byte("test")}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 5, byte(wire.ActReplBody), 't', 'e', 's', 't', 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActReplBody accept up to max size", with256KbC(withActC(withProtC(0), OptChangeBody)), ops{
			{s1: sendConnect, v1: func(t *testing.T, s *ClientSession, act *Action, err error) {
				expectContinue(t, s, act, err)
				if s.negotiatedBodySize != uint32(DataSize256K) {
					t.Fatalf("buffer size: expected: %d, got %d", DataSize256K, s.negotiatedBodySize)
				}
			}, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				// expectErr1(t, s, act, err)
			}, server: func() []byte {
				data := make([]byte, DataSize256K)
				r := []byte{0, 0, 0, 0, byte(wire.ActReplBody)}
				binary.BigEndian.PutUint32(r, uint32(1+len(data)))
				r = append(r, data...)
				r = append(r, 0, 0, 0, 1, byte(wire.ActAccept))
				return r
			}()},
		}},
		{"ActReplBody enforce max size", withActC(withProtC(0), OptChangeBody), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: func() []byte {
				data := make([]byte, DataSize64K+1)
				r := []byte{0, 0, 0, 0, byte(wire.ActReplBody)}
				binary.BigEndian.PutUint32(r, uint32(1+len(data)))
				r = append(r[:], data...)
				return r
			}()},
		}},
		{"ActChangeFrom working 1", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeFrom, From: "<>"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 4, byte(wire.ActChangeFrom), '<', '>', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActChangeFrom working 2", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeFrom, From: "<>", FromArgs: "A"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 6, byte(wire.ActChangeFrom), '<', '>', 0, 'A', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActChangeFrom error detection 1", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 3, byte(wire.ActChangeFrom), '<', '>', 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActChangeFrom error detection 2", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 8, byte(wire.ActChangeFrom), '<', '>', 0, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActChangeHeader working", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeHeader, HeaderIndex: 3, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 9, byte(wire.ActChangeHeader), 0, 0, 0, 3, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActChangeHeader error detection 1", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 4, byte(wire.ActChangeHeader), 0, 0, 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActChangeHeader error detection 2", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 5, byte(wire.ActChangeHeader), 0, 0, 0, 3, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActChangeHeader error detection 3", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 7, byte(wire.ActChangeHeader), 0, 0, 0, 3, 'A', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActInsertHeader working", withActC(withProtC(0), OptAddHeader), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionInsertHeader, HeaderIndex: 3, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 9, byte(wire.ActInsertHeader), 0, 0, 0, 3, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"ActInsertHeader working", withActC(withProtC(0), OptAddHeader), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, _ *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionAddHeader, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 5, byte(wire.ActAddHeader), 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"End Unknown msg code", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 1, 'O', 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"Two messages in a row", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeHeader, HeaderIndex: 3, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 9, byte(wire.ActChangeHeader), 0, 0, 0, 3, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeHeader, HeaderIndex: 3, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 9, byte(wire.ActChangeHeader), 0, 0, 0, 3, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"Two connections in a row", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeHeader, HeaderIndex: 3, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 9, byte(wire.ActChangeHeader), 0, 0, 0, 3, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
			{s2: func(s *ClientSession) error {
				return s.Reset(nil)
			}, v2: func(t *testing.T, s *ClientSession, err error) {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}, server: nil},
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeHeader, HeaderIndex: 3, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 9, byte(wire.ActChangeHeader), 0, 0, 0, 3, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"no Reset after error", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 1, 'O', 0, 0, 0, 1, byte(wire.ActAccept)}},
			{s2: func(s *ClientSession) error {
				return s.Reset(nil)
			}, v2: func(t *testing.T, s *ClientSession, err error) {
				if err == nil {
					t.Fatalf("expected error")
				}
			}, server: nil},
		}},
		{"Abort after Rcpt", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s2: func(s *ClientSession) error {
				return s.Abort(nil)
			}, v2: func(t *testing.T, s *ClientSession, err error) {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}, server: nil},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderField, v1: expectContinue, server: responseContinue},
			{s1: sendHeaderEnd, v1: expectContinue, server: responseContinue},
			{s1: sendBodyChunk, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectAct(ActionAccept, t, act, err)
				exp := []ModifyAction{{Type: ActionChangeHeader, HeaderIndex: 3, HeaderName: "A", HeaderValue: "B"}}
				if !reflect.DeepEqual(exp, mActs) {
					t.Fatalf("modifications: expect %+v, got %+v", exp, mActs)
				}
			}, server: []byte{0, 0, 0, 9, byte(wire.ActChangeHeader), 0, 0, 0, 3, 'A', 0, 'B', 0, 0, 0, 0, 1, byte(wire.ActAccept)}},
		}},
		{"no Abort after error", withActC(withProtC(0), OptChangeFrom), ops{
			{s1: sendConnect, v1: expectContinue, server: responseContinue},
			{s1: sendHelo, v1: expectContinue, server: responseContinue},
			{s1: sendMail, v1: expectContinue, server: responseContinue},
			{s1: sendRcpt, v1: expectContinue, server: responseContinue},
			{s1: sendData, v1: expectContinue, server: responseContinue},
			{s3: sendEnd, v3: func(t *testing.T, s *ClientSession, mActs []ModifyAction, act *Action, err error) {
				expectErr1(t, s, act, err)
			}, server: []byte{0, 0, 0, 1, 'O', 0, 0, 0, 1, byte(wire.ActAccept)}},
			{s2: func(s *ClientSession) error {
				return s.Abort(nil)
			}, v2: func(t *testing.T, s *ClientSession, err error) {
				if err == nil {
					t.Fatalf("expected error")
				}
			}, server: nil},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			// just slurp up everything the client sends
			go func() {
				buf := make([]byte, 1024)
				for {
					_ = serverConn.SetReadDeadline(time.Now().Add(time.Minute))
					if _, err := serverConn.Read(buf); err != nil {
						if err != io.EOF && err != io.ErrClosedPipe {
							t.Logf("server got error: read: %v", err)
						}
						return
					}
				}
			}()
			// send pre-defined answers to the client
			go func() {
				if _, err := serverConn.Write(ltt.cfg.ServerNegotiation); err != nil {
					t.Logf("server got error: write: %v", err)
					return
				}
				for _, op := range ltt.ops {
					if op.server == nil {
						continue
					}
					if _, err := serverConn.Write(op.server); err != nil {
						if err != io.ErrClosedPipe {
							t.Logf("server got error: write: %v", err)
						}
						return
					}
				}
			}()
			cl := NewClient(clientConn.LocalAddr().Network(), clientConn.LocalAddr().String(), ltt.cfg.Opts...)
			session, err := cl.session(clientConn, ltt.cfg.Macros)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			for i, op := range ltt.ops {
				t.Logf("%q op %d", ltt.name, i)
				if op.s1 != nil {
					act, err := op.s1(session)
					op.v1(t, session, act, err)
				} else if op.s2 != nil {
					op.v2(t, session, op.s2(session))
				} else if op.s3 != nil {
					mActs, act, err := op.s3(session)
					op.v3(t, session, mActs, act, err)
				} else {
					panic("one of s1, s2 or s3 must be set")
				}
			}
		})
	}
}
