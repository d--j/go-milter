package milter

import (
	"bytes"
	"errors"
	"net/textproto"
	"reflect"
	"testing"

	"github.com/d--j/go-milter/internal/wire"
)

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
				t.Errorf("processMsg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var got *wire.Message
			if gotR != nil {
				got = gotR.Response()
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processMsg() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type processTestMilter struct {
	respContinue      *Response
	err               error
	initCalled        int
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

func (p *processTestMilter) NewConnection(m Modifier) error {
	p.initCalled++
	return p.err
}

func (p *processTestMilter) Connect(host string, family string, port uint16, addr string, m Modifier) (*Response, error) {
	p.host = host
	p.family = family
	p.port = port
	p.addr = addr
	return p.respContinue, p.err
}

func (p *processTestMilter) Helo(name string, m Modifier) (*Response, error) {
	p.name = name
	return p.respContinue, p.err
}

func (p *processTestMilter) MailFrom(from string, esmtpArgs string, m Modifier) (*Response, error) {
	p.from = from
	p.fromEsmtp = esmtpArgs
	return p.respContinue, p.err
}

func (p *processTestMilter) RcptTo(rcptTo string, esmtpArgs string, m Modifier) (*Response, error) {
	p.rcptTo = rcptTo
	p.rcptEsmtp = esmtpArgs
	return p.respContinue, p.err
}

func (p *processTestMilter) Data(m Modifier) (*Response, error) {
	p.dataCalled = true
	return p.respContinue, p.err
}

func (p *processTestMilter) Header(name string, value string, m Modifier) (*Response, error) {
	p.hdrName = name
	p.hdrValue = value
	return p.respContinue, p.err
}

func (p *processTestMilter) Headers(m Modifier) (*Response, error) {
	p.headersCalled = true
	return p.respContinue, p.err
}

func (p *processTestMilter) BodyChunk(chunk []byte, m Modifier) (*Response, error) {
	p.chunk = chunk
	return p.respContinue, p.err
}

func (p *processTestMilter) EndOfMessage(m Modifier) (*Response, error) {
	p.eomCalled = true
	return RespAccept, p.err
}

func (p *processTestMilter) Unknown(cmd string, m Modifier) (*Response, error) {
	p.cmd = cmd
	return p.respContinue, p.err
}

func (p *processTestMilter) Abort(m Modifier) error {
	p.abortCalled = true
	return p.err
}

func (p *processTestMilter) Cleanup(m Modifier) {
	p.cleanupCalled++
}

var _ Milter = (*processTestMilter)(nil)

type processTestMilterStack struct {
	backends []*processTestMilter
}

func (p *processTestMilterStack) last() *processTestMilter {
	if len(p.backends) == 0 {
		return nil
	}
	return p.backends[len(p.backends)-1]
}

type processTestMilterFactory func() (func() Milter, *processTestMilterStack)

func Test_milterSession_processMsg(t *testing.T) {
	def := func() (func() Milter, *processTestMilterStack) {
		stack := &processTestMilterStack{}
		return func() Milter {
			m := &processTestMilter{respContinue: RespContinue}
			stack.backends = append(stack.backends, m)
			return m
		}, stack
	}
	err := func() (func() Milter, *processTestMilterStack) {
		stack := &processTestMilterStack{}
		return func() Milter {
			m := &processTestMilter{respContinue: RespContinue, err: errors.New("init error")}
			stack.backends = append(stack.backends, m)
			return m
		}, stack
	}
	acceptAll := func() (func() Milter, *processTestMilterStack) {
		stack := &processTestMilterStack{}
		return func() Milter {
			m := &processTestMilter{respContinue: RespAccept}
			stack.backends = append(stack.backends, m)
			return m
		}, stack
	}

	type fields struct {
		actions  OptAction
		protocol OptProtocol
		backend  processTestMilterFactory
		check    func(*testing.T, *serverSession, *processTestMilterStack)
	}
	cont := &wire.Message{Code: wire.Code(wire.ActContinue)}
	accept := &wire.Message{Code: wire.Code(wire.ActAccept)}

	tests := []struct {
		name    string
		fields  fields
		msg     *wire.Message
		wantMsg *wire.Message
		wantErr bool
	}{
		{"no-double-neg", fields{
			backend: def,
		}, &wire.Message{Code: wire.CodeOptNeg}, nil, true},
		{"abort", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if !stack.last().abortCalled {
					t.Fatalf("abort not called")
				}
			},
		}, &wire.Message{Code: wire.CodeAbort}, nil, false},
		{"abort-2", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if !stack.last().abortCalled {
					t.Fatalf("abort not called")
				}
			}}, &wire.Message{Code: wire.CodeAbort}, nil, false},
		{"abort-3", fields{backend: err}, &wire.Message{Code: wire.CodeAbort}, nil, true},
		{"quit-new-conn", fields{
			backend: def,
		}, &wire.Message{wire.CodeQuitNewConn, nil}, nil, false},
		{"quit", fields{
			backend: def,
		}, &wire.Message{wire.CodeQuit, nil}, nil, false},
		{"unknown-code", fields{
			backend: def,
		}, &wire.Message{wire.Code('@'), nil}, nil, true},
		{"conn err 1", fields{backend: err}, &wire.Message{wire.CodeConn, []byte{'h', 0, 'L', 0, 0, '/', 'r', 'u', 'n', 0}}, cont, true},
		{"conn err 2", fields{backend: def}, &wire.Message{wire.CodeConn, nil}, nil, true},
		{"conn unknown protocol", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				p := stack.last()
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
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				p := stack.last()
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
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				p := stack.last()
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
			backend: def,
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '4', 9, 251, '6', '6', '6', '.', '0', '.', '0', '.', '1', '2', 0}}, nil, true},
		{"conn tcp6 protocol", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				p := stack.last()
				if p.family != "tcp6" {
					t.Errorf("expected tcp6, got %q", p.family)
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
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				p := stack.last()
				if p.family != "tcp6" {
					t.Errorf("expected tcp6, got %q", p.family)
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
		{"conn tcp6 protocol 3", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				p := stack.last()
				if p.family != "tcp6" {
					t.Errorf("expected tcp6, got %q", p.family)
				}
				if p.addr != "::1" {
					t.Errorf("expected ::1, got %q", p.addr)
				}
				if p.port != 2555 {
					t.Errorf("expected 2555, got %v", p.port)
				}
				if p.host != "h" {
					t.Errorf("expected \"h\", got %q", p.host)
				}
			},
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '6', 9, 251, 'I', 'P', 'v', '6', ':', '0', ':', '0', ':', '0', ':', '0', ':', '0', ':', '0', ':', '0', ':', '1', 0}}, cont, false},
		{"conn tcp6 protocol err", fields{
			backend: def,
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '6', 9, 251, '[', '@', ']', 0}}, nil, true},
		{"conn tcp6 protocol err 2", fields{
			backend: def,
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '6', 9}}, nil, true},
		{"conn bogus protocol err", fields{
			backend: def,
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '+', 9, 251, '[', ':', ':', ']', 0}}, nil, true},
		{"conn accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeConn, []byte{'h', 0, '4', 9, 251, '1', '2', '7', '.', '0', '.', '0', '.', '1', '2', 0}}, accept, false},
		{"helo", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().name != "h" {
					t.Errorf("expected h, got %q", stack.last().name)
				}
			},
		}, &wire.Message{wire.CodeHelo, []byte{'h', 0}}, cont, false},
		{"helo accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeHelo, []byte{'h', 0}}, accept, false},
		{"helo-err-1", fields{backend: err}, &wire.Message{wire.CodeHelo, []byte{'h', 0}}, cont, true},
		{"helo-err-2", fields{
			backend: def,
		}, &wire.Message{wire.CodeHelo, []byte{}}, nil, true},
		{"mail", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().from != "r" {
					t.Errorf("expected r, got %q", stack.last().from)
				}
				if stack.last().fromEsmtp != "" {
					t.Errorf("expected \"\", got %q", stack.last().fromEsmtp)
				}
			},
		}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0}}, cont, false},
		{"mail accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0}}, accept, false},
		{"mail esmtp", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().from != "r" {
					t.Errorf("expected r, got %q", stack.last().from)
				}
				if stack.last().fromEsmtp != "A=B" {
					t.Errorf("expected A=B, got %q", stack.last().fromEsmtp)
				}
			},
		}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0, 'A', '=', 'B', 0}}, cont, false},
		{"mail esmtp multi", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().from != "r" {
					t.Errorf("expected r, got %q", stack.last().from)
				}
				if stack.last().fromEsmtp != "A=B C=D" {
					t.Errorf("expected A=B C=D, got %q", stack.last().fromEsmtp)
				}
			},
		}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0, 'A', '=', 'B', 0, 'C', '=', 'D', 0}}, cont, false},
		{"mail-err-1", fields{backend: err}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0}}, cont, true},
		{"mail-err-2", fields{
			backend: def,
		}, &wire.Message{wire.CodeMail, []byte{}}, nil, true},
		{"mail-err-3", fields{
			backend: err,
		}, &wire.Message{wire.CodeMail, []byte{'<', 'r', '>', 0, 'A', '=', 'B', 0}}, cont, true},
		{"rcpt", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().rcptTo != "r" {
					t.Errorf("expected r, got %q", stack.last().rcptTo)
				}
				if stack.last().rcptEsmtp != "" {
					t.Errorf("expected \"\", got %q", stack.last().rcptEsmtp)
				}
			},
		}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0}}, cont, false},
		{"rcpt accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0}}, accept, false},
		{"rcpt-repeat", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().rcptTo != "r" {
					t.Errorf("expected r, got %q", stack.last().rcptTo)
				}
				if stack.last().rcptEsmtp != "" {
					t.Errorf("expected \"\", got %q", stack.last().rcptEsmtp)
				}
			},
		}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0}}, cont, false},
		{"rcpt esmtp", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().rcptTo != "r" {
					t.Errorf("expected r, got %q", stack.last().rcptTo)
				}
				if stack.last().rcptEsmtp != "A=B" {
					t.Errorf("expected A=B, got %q", stack.last().rcptEsmtp)
				}
			},
		}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0, 'A', '=', 'B', 0}}, cont, false},
		{"rcpt esmtp multi", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().rcptTo != "r" {
					t.Errorf("expected r, got %q", stack.last().rcptTo)
				}
				if stack.last().rcptEsmtp != "A=B C=D" {
					t.Errorf("expected A=B C=D, got %q", stack.last().rcptEsmtp)
				}
			},
		}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0, 'A', '=', 'B', 0, 'C', '=', 'D', 0}}, cont, false},
		{"rcpt-err-1", fields{backend: err}, &wire.Message{wire.CodeRcpt, []byte{'<', 'r', '>', 0}}, cont, true},
		{"rcpt-err-2", fields{
			backend: def,
		}, &wire.Message{wire.CodeRcpt, []byte{}}, nil, true},
		{"data", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if !stack.last().dataCalled {
					t.Errorf("expected dataCalled false")
				}
			},
		}, &wire.Message{wire.CodeData, nil}, cont, false},
		{"data accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeData, nil}, accept, false},
		{"data-err-1", fields{backend: err}, &wire.Message{wire.CodeData, nil}, cont, true},
		{"header", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().hdrName != "To" {
					t.Errorf("expected To, got %q", stack.last().hdrName)
				}
				if stack.last().hdrValue != "<>" {
					t.Errorf("expected <>, got %q", stack.last().hdrName)
				}
			},
		}, &wire.Message{wire.CodeHeader, []byte{'T', 'o', 0, '<', '>', 0}}, cont, false},
		{"header accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeHeader, []byte{'T', 'o', 0, '<', '>', 0}}, accept, false},
		{"header-err-1", fields{backend: err}, &wire.Message{wire.CodeHeader, []byte{'T', 'o', 0, '<', '>', 0}}, cont, true},
		{"header-err-2", fields{
			backend: def,
		}, &wire.Message{wire.CodeHeader, []byte{'T', 'o', 0}}, nil, true},
		{"header-err-3", fields{
			backend: def,
		}, &wire.Message{wire.CodeHeader, []byte{}}, nil, true},
		{"eoh", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if !stack.last().headersCalled {
					t.Errorf("Headers() not called")
				}
			},
		}, &wire.Message{wire.CodeEOH, nil}, cont, false},
		{"eoh accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeEOH, nil}, accept, false},
		{"eoh-err-1", fields{backend: err}, &wire.Message{wire.CodeEOH, nil}, cont, true},
		{"body empty", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if stack.last().chunk == nil || len(stack.last().chunk) > 0 {
					t.Errorf("expected \"\", got %q", stack.last().chunk)
				}
			},
		}, &wire.Message{wire.CodeBody, []byte{}}, cont, false},
		{"body", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if !bytes.Equal(stack.last().chunk, []byte("abc")) {
					t.Errorf("expected \"abc\", got %q", stack.last().chunk)
				}
			},
		}, &wire.Message{wire.CodeBody, []byte{'a', 'b', 'c'}}, cont, false},
		{"body accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeBody, []byte{'a', 'b', 'c'}}, accept, false},
		{"body-err-1", fields{backend: err}, &wire.Message{wire.CodeBody, []byte{'a', 'b', 'c'}}, cont, true},
		{"end", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if !stack.last().eomCalled {
					t.Errorf("EndOfMessage() not called")
				}
			},
		}, &wire.Message{wire.CodeEOB, []byte{'a', 'b', 'c'}}, &wire.Message{wire.Code(wire.ActAccept), nil}, false},
		{"end-err-1", fields{backend: err}, &wire.Message{wire.CodeEOB, []byte{'a', 'b', 'c'}}, accept, true},
		{"unknown", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				p := stack.last()
				if p.cmd != "abc" {
					t.Errorf("expected abc, got %q", p.cmd)
				}
			},
		}, &wire.Message{wire.CodeUnknown, []byte{'a', 'b', 'c', 0}}, cont, false},
		{"unknown accept", fields{
			backend: acceptAll,
		}, &wire.Message{wire.CodeUnknown, []byte{'a', 'b', 'c', 0}}, accept, false},
		{"unknown-err-1", fields{
			backend: err,
		}, &wire.Message{wire.CodeUnknown, []byte{'a', 'b', 'c', 0}}, cont, true},
		{"macro 1", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				if s.macros.byStages[StageConnect] != nil {
					t.Errorf("should be <nil>")
				}
			},
		}, &wire.Message{wire.CodeMacro, []byte{byte(wire.CodeConn)}}, nil, false},
		{"macro 2", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				expect := map[MacroName]string{
					MacroMTAFQDN: "1",
					MacroQueueId: "2",
				}
				if !reflect.DeepEqual(expect, s.macros.byStages[StageConnect]) {
					t.Errorf("expect %+v, got %+v", expect, s.macros.byStages[StageConnect])
				}
			},
		}, &wire.Message{wire.CodeMacro, []byte{byte(wire.CodeConn), 'j', 0, '1', 0, 'i', 0, '2', 0}}, nil, false},
		{"macro 3", fields{
			backend: def,
			check: func(t *testing.T, s *serverSession, stack *processTestMilterStack) {
				expect := map[MacroName]string{
					MacroMTAFQDN: "1",
					MacroQueueId: "",
				}
				if !reflect.DeepEqual(expect, s.macros.byStages[StageConnect]) {
					t.Errorf("expect %+v, got %+v", expect, s.macros.byStages[StageConnect])
				}
			},
		}, &wire.Message{wire.CodeMacro, []byte{byte(wire.CodeConn), 'j', 0, '1', 0, 'i', 0}}, nil, false},
		{"macro err", fields{
			backend: def,
		}, &wire.Message{wire.CodeMacro, []byte{}}, nil, true},
	}
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			factory, stack := tt.fields.backend()
			s := NewServer(WithMilter(factory))
			m := &serverSession{
				server:   s,
				version:  MaxServerProtocolVersion,
				actions:  tt.fields.actions,
				protocol: tt.fields.protocol,
				macros:   newMacroStages(),
			}
			m.modifier = newModifier(m, modifierStateReadOnly)
			gotR, err := m.processMsg(factory(), tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("processMsg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var got *wire.Message
			if gotR != nil {
				got = gotR.Response()
			}
			if !reflect.DeepEqual(got, tt.wantMsg) {
				t.Errorf("processMsg() got = %v, want %v", got, tt.wantMsg)
			}
			if tt.fields.check != nil {
				tt.fields.check(t, m, stack)
			}
		})
	}
}
