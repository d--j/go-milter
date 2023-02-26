package milter

import (
	"bytes"
	"errors"
	"net/textproto"
	"reflect"
	"testing"

	"github.com/d--j/go-milter/internal/wire"
)

type processTestMilter struct {
	cleanupCalled     int
	host              string
	family            string
	port              uint16
	addr              string
	name              string
	from              string
	fromEsmtp         string
	rcptTo            string
	rcptEsmtp         string
	dataCalled        bool
	hdrName, hdrValue string
	headers           textproto.MIMEHeader
	headersCalled     bool
	chunk             []byte
	eomCalled         bool
	abortCalled       bool
	cmd               string
}

func (p *processTestMilter) Connect(host string, family string, port uint16, addr string, m *Modifier) (*Response, error) {
	p.host = host
	p.family = family
	p.port = port
	p.addr = addr
	return RespContinue, nil
}

func (p *processTestMilter) Helo(name string, m *Modifier) (*Response, error) {
	p.name = name
	return RespContinue, nil
}

func (p *processTestMilter) MailFrom(from string, esmtpArgs string, m *Modifier) (*Response, error) {
	p.from = from
	p.fromEsmtp = esmtpArgs
	return RespContinue, nil
}

func (p *processTestMilter) RcptTo(rcptTo string, esmtpArgs string, m *Modifier) (*Response, error) {
	p.rcptTo = rcptTo
	p.rcptEsmtp = esmtpArgs
	return RespContinue, nil
}

func (p *processTestMilter) Data(m *Modifier) (*Response, error) {
	p.dataCalled = true
	return RespContinue, nil
}

func (p *processTestMilter) Header(name string, value string, m *Modifier) (*Response, error) {
	p.hdrName = name
	p.hdrValue = value
	return RespContinue, nil
}

func (p *processTestMilter) Headers(m *Modifier) (*Response, error) {
	p.headers = m.Headers
	p.headersCalled = true
	return RespContinue, nil
}

func (p *processTestMilter) BodyChunk(chunk []byte, m *Modifier) (*Response, error) {
	p.chunk = chunk
	return RespContinue, nil
}

func (p *processTestMilter) EndOfMessage(m *Modifier) (*Response, error) {
	p.eomCalled = true
	return RespAccept, nil
}

func (p *processTestMilter) Abort(_ *Modifier) error {
	p.abortCalled = true
	return nil
}

func (p *processTestMilter) Unknown(cmd string, m *Modifier) (*Response, error) {
	p.cmd = cmd
	return RespContinue, nil
}

func (p *processTestMilter) Cleanup() {
	p.cleanupCalled++
}

var _ Milter = &processTestMilter{}

func Test_milterSession_negotiate(t *testing.T) {
	type fields struct {
		milterVersion  uint32
		milterActions  OptAction
		milterProtocol OptProtocol
		callback       NegotiationCallbackFunc
		macroRequests  macroRequests
	}

	tests := []struct {
		name    string
		fields  fields
		msg     *wire.Message
		want    *wire.Message
		wantErr bool
	}{
		{"negotiation error 1", fields{}, &wire.Message{wire.CodeOptNeg, nil}, nil, true},
		{"negotiation error 2", fields{}, &wire.Message{wire.CodeOptNeg, []byte{0, 0, 0, 99, 0, 0, 0, 0, 0, 0, 0, 0}}, nil, true},
		{"negotiation error 3", fields{}, &wire.Message{wire.CodeOptNeg, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}, nil, true},
		{"negotiation error 4", fields{callback: func(mtaVersion, milterVersion uint32, mtaActions, milterActions OptAction, mtaProtocol, milterProtocol OptProtocol, offeredMaxData DataSize) (version uint32, actions OptAction, protocol OptProtocol, maxData DataSize, err error) {
			return 0, 0, 0, 0, errors.New("error")
		}}, &wire.Message{wire.CodeOptNeg, []byte{0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0}}, nil, true},
		{"negotiation", fields{callback: func(mtaVersion, milterVersion uint32, mtaActions, milterActions OptAction, mtaProtocol, milterProtocol OptProtocol, offeredMaxData DataSize) (version uint32, actions OptAction, protocol OptProtocol, maxData DataSize, err error) {
			return milterVersion, OptAddHeader, OptNoConnect, DataSize64K, nil
		}}, &wire.Message{wire.CodeOptNeg, []byte{0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0}}, &wire.Message{wire.CodeOptNeg, []byte{0, 0, 0, 6, 0, 0, 0, 1, 0, 0, 0, 1}}, false},
		{"negotiation macros", fields{milterActions: OptSetMacros, macroRequests: macroRequests{{"j", "_"}, {"i"}}}, &wire.Message{wire.CodeOptNeg, []byte{0, 0, 0, 2, 0, 0, 1, 0, 0, 0, 0, 0}}, &wire.Message{wire.CodeOptNeg, []byte{0, 0, 0, 2, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'j', ' ', '_', 0, 0, 0, 0, 1, 'i', 0}}, false},
	}
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			m := &serverSession{}
			milterVersion := tt.fields.milterVersion
			if milterVersion == 0 {
				milterVersion = MaxServerProtocolVersion
			}
			gotR, err := m.negotiate(tt.msg, milterVersion, tt.fields.milterActions, tt.fields.milterProtocol, tt.fields.callback, tt.fields.macroRequests, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var got *wire.Message
			if gotR != nil {
				got = gotR.Response()
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Process() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_milterSession_Process(t *testing.T) {
	type fields struct {
		actions  OptAction
		protocol OptProtocol
		backend  Milter
		check    func(*testing.T, *serverSession)
	}
	cont := &wire.Message{wire.Code(wire.ActContinue), nil}

	tests := []struct {
		name    string
		fields  fields
		msg     *wire.Message
		want    *wire.Message
		wantErr bool
	}{
		{"abort", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.cleanupCalled != 0 {
					t.Errorf("Cleanup() called %d times", p.cleanupCalled)
				}
				if !p.abortCalled {
					t.Errorf("Abort() not called")
				}
			},
		}, &wire.Message{wire.CodeAbort, nil}, nil, false},
		{"quit-new-conn", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				if s.backend.(*processTestMilter).cleanupCalled != 1 {
					t.Fatalf("Cleanup() called %d times", s.backend.(*processTestMilter).cleanupCalled)
				}
			},
		}, &wire.Message{wire.CodeQuitNewConn, nil}, nil, false},
		{"quit", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				if s.backend.(*processTestMilter).cleanupCalled != 1 {
					t.Fatalf("Cleanup() called %d times", s.backend.(*processTestMilter).cleanupCalled)
				}
			},
		}, &wire.Message{wire.CodeQuit, nil}, nil, true},
		{"unknown", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.Code('@'), nil}, nil, true},
		{"conn err 1", fields{backend: &processTestMilter{}}, &wire.Message{wire.CodeConn, nil}, nil, true},
		{"conn unknown protocol", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.family != "unknown" {
					t.Errorf("expected unknown, got %q", p.family)
				}
				if p.addr != "" {
					t.Errorf("expected \"\", got %q", p.addr)
				}
				if p.port != 0 {
					t.Errorf("expected 0, got %v", p.port)
				}
				if p.host != "" {
					t.Errorf("expected \"\", got %q", p.host)
				}
			},
		}, &wire.Message{wire.CodeConn, []byte{0, 'U'}}, cont, false},
		{"conn unix protocol", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.family != "unix" {
					t.Errorf("expected unix, got %q", p.family)
				}
				if p.addr != "/run" {
					t.Errorf("expected /run, got %q", p.addr)
				}
				if p.port != 0 {
					t.Errorf("expected 0, got %v", p.port)
				}
				if p.host != "h" {
					t.Errorf("expected \"h\", got %q", p.host)
				}
			},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, 'L', 0, 0, '/', 'r', 'u', 'n', 0}}, cont, false},
		{"conn tcp4 protocol", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.family != "tcp4" {
					t.Errorf("expected tcp4, got %q", p.family)
				}
				if p.addr != "127.0.0.12" {
					t.Errorf("expected 127.0.0.12, got %q", p.addr)
				}
				if p.port != 2555 {
					t.Errorf("expected 2555, got %v", p.port)
				}
				if p.host != "h" {
					t.Errorf("expected \"h\", got %q", p.host)
				}
			},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '4', 9, 251, '1', '2', '7', '.', '0', '.', '0', '.', '1', '2', 0}}, cont, false},
		{"conn tcp4 protocol err", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '4', 9, 251, '6', '6', '6', '.', '0', '.', '0', '.', '1', '2', 0}}, nil, true},
		{"conn tcp6 protocol", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.family != "tcp6" {
					t.Errorf("expected tcp4, got %q", p.family)
				}
				if p.addr != "::" {
					t.Errorf("expected ::, got %q", p.addr)
				}
				if p.port != 2555 {
					t.Errorf("expected 2555, got %v", p.port)
				}
				if p.host != "h" {
					t.Errorf("expected \"h\", got %q", p.host)
				}
			},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '6', 9, 251, ':', ':', 0}}, cont, false},
		{"conn tcp6 protocol 2", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.family != "tcp6" {
					t.Errorf("expected tcp4, got %q", p.family)
				}
				if p.addr != "::" {
					t.Errorf("expected ::, got %q", p.addr)
				}
				if p.port != 2555 {
					t.Errorf("expected 2555, got %v", p.port)
				}
				if p.host != "h" {
					t.Errorf("expected \"h\", got %q", p.host)
				}
			},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '6', 9, 251, '[', ':', ':', ']', 0}}, cont, false},
		{"conn tcp6 protocol err", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '6', 9, 251, '[', '@', ']', 0}}, nil, true},
		{"conn tcp6 protocol err 2", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '6', 9}}, nil, true},
		{"conn bogus protocol err", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '+', 9, 251, '[', ':', ':', ']', 0}}, nil, true},
		{"helo", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.name != "h" {
					t.Errorf("expected h, got %q", p.name)
				}
			},
		}, &wire.Message{wire.CodeHelo, []byte{'h', 0}}, cont, false},
		{"helo err", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeHelo, []byte{}}, nil, true},
		{"mail", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.from != "r" {
					t.Errorf("expected r, got %q", p.from)
				}
				if p.fromEsmtp != "" {
					t.Errorf("expected \"\", got %q", p.fromEsmtp)
				}
			},
		}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0}}, cont, false},
		{"mail esmtp", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.from != "r" {
					t.Errorf("expected r, got %q", p.from)
				}
				if p.fromEsmtp != "A=B" {
					t.Errorf("expected A=B, got %q", p.fromEsmtp)
				}
			},
		}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0, 'A', '=', 'B', 0}}, cont, false},
		{"mail err", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeMail, []byte{}}, nil, true},
		{"rcpt", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.rcptTo != "r" {
					t.Errorf("expected r, got %q", p.rcptTo)
				}
				if p.rcptEsmtp != "" {
					t.Errorf("expected \"\", got %q", p.rcptEsmtp)
				}
			},
		}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0}}, cont, false},
		{"rcpt esmtp", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.rcptTo != "r" {
					t.Errorf("expected r, got %q", p.rcptTo)
				}
				if p.rcptEsmtp != "A=B" {
					t.Errorf("expected A=B, got %q", p.rcptEsmtp)
				}
			},
		}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0, 'A', '=', 'B', 0}}, cont, false},
		{"rcpt err", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeRcpt, []byte{}}, nil, true},
		{"data", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if !p.dataCalled {
					t.Errorf("expected dataCalled true")
				}
			},
		}, &wire.Message{wire.CodeData, nil}, cont, false},
		{"header", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.hdrName != "To" {
					t.Errorf("expected To, got %q", p.hdrName)
				}
				if p.hdrValue != "<>" {
					t.Errorf("expected <>, got %q", p.hdrName)
				}
			},
		}, &wire.Message{wire.CodeHeader, []byte{'T', 'o', 0, '<', '>', 0}}, cont, false},
		{"header err 1", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeHeader, []byte{'T', 'o', 0}}, nil, true},
		{"header err 2", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeHeader, []byte{}}, nil, true},
		{"eoh", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if !p.headersCalled {
					t.Errorf("Headers() not called")
				}
			},
		}, &wire.Message{wire.CodeEOH, nil}, cont, false},
		{"body empty", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.chunk == nil || len(p.chunk) > 0 {
					t.Errorf("expected \"\", got %q", p.chunk)
				}
			},
		}, &wire.Message{wire.CodeBody, []byte{}}, cont, false},
		{"body", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if !bytes.Equal(p.chunk, []byte("abc")) {
					t.Errorf("expected \"abc\", got %q", p.chunk)
				}
			},
		}, &wire.Message{wire.CodeBody, []byte{'a', 'b', 'c'}}, cont, false},
		{"end", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if !p.eomCalled {
					t.Errorf("EndOfMessage() not called")
				}
			},
		}, &wire.Message{wire.CodeEOB, []byte{'a', 'b', 'c'}}, &wire.Message{wire.Code(wire.ActAccept), nil}, false},
		{"unknown", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				p := s.backend.(*processTestMilter)
				if p.cmd != "abc" {
					t.Errorf("expected abc, got %q", p.cmd)
				}
			},
		}, &wire.Message{wire.CodeUnknown, []byte{'a', 'b', 'c', 0}}, cont, false},
		{"macro 1", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				if s.macros.byStages[StageConnect] != nil {
					t.Errorf("should be <nil>")
				}
			},
		}, &wire.Message{wire.CodeMacro, []byte{byte(wire.CodeConn)}}, nil, false},
		{"macro 2", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				expect := map[MacroName]string{
					MacroMTAFullyQualifiedDomainName: "1",
					MacroQueueId:                     "2",
				}
				if !reflect.DeepEqual(expect, s.macros.byStages[StageConnect]) {
					t.Errorf("expect %+v, got %+v", expect, s.macros.byStages[StageConnect])
				}
			},
		}, &wire.Message{wire.CodeMacro, []byte{byte(wire.CodeConn), 'j', 0, '1', 0, 'i', 0, '2', 0}}, nil, false},
		{"macro 3", fields{
			backend: &processTestMilter{},
			check: func(t *testing.T, s *serverSession) {
				expect := map[MacroName]string{
					MacroMTAFullyQualifiedDomainName: "1",
					MacroQueueId:                     "",
				}
				if !reflect.DeepEqual(expect, s.macros.byStages[StageConnect]) {
					t.Errorf("expect %+v, got %+v", expect, s.macros.byStages[StageConnect])
				}
			},
		}, &wire.Message{wire.CodeMacro, []byte{byte(wire.CodeConn), 'j', 0, '1', 0, 'i', 0}}, nil, false},
		{"macro err", fields{
			backend: &processTestMilter{},
		}, &wire.Message{wire.CodeMacro, []byte{}}, nil, true},
	}
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			s := NewServer(WithMilter(func() Milter {
				return tt.fields.backend
			}))
			m := &serverSession{
				server:   s,
				version:  MaxServerProtocolVersion,
				actions:  tt.fields.actions,
				protocol: tt.fields.protocol,
				macros:   newMacroStages(),
				backend:  tt.fields.backend,
			}
			gotR, err := m.Process(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var got *wire.Message
			if gotR != nil {
				got = gotR.Response()
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Process() got = %v, want %v", got, tt.want)
			}
			if tt.fields.check != nil {
				tt.fields.check(t, m)
			}
		})
	}
}
