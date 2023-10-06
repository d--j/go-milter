package milter

import (
	"bytes"
	"github.com/d--j/go-milter/internal/wire"
	"github.com/emersion/go-message/textproto"
	"net"
	"testing"
)

func TestNoOpMilter(t *testing.T) {
	t.Parallel()
	asset := func(resp *Response, err error, act wire.ActionCode) {
		t.Helper()
		if resp.Response().Code != wire.Code(act) {
			t.Fatalf("NoOpMilter response is not %c: %+v", act, resp)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	assetContinue := func(resp *Response, err error) {
		t.Helper()
		asset(resp, err, wire.ActContinue)
	}
	assetAccept := func(resp *Response, err error) {
		t.Helper()
		asset(resp, err, wire.ActAccept)
	}
	m := NoOpMilter{}
	assetContinue(m.Connect("", "", 0, "", nil))
	assetContinue(m.Helo("", nil))
	assetContinue(m.MailFrom("", "", nil))
	assetContinue(m.RcptTo("", "", nil))
	assetContinue(m.Data(nil))
	assetContinue(m.Header("", "", nil))
	assetContinue(m.Headers(nil))
	assetContinue(m.BodyChunk(nil, nil))
	assetAccept(m.EndOfMessage(nil))
	assetContinue(m.Unknown("", nil))
	if err := m.Abort(nil); err != nil {
		t.Fatal(err)
	}
}

func TestServer_NoOpMilter(t *testing.T) {
	t.Parallel()
	assert := func(act *Action, err error, expectedCode ActionType) {
		t.Helper()
		if err != nil {
			t.Fatalf("got err: %v", err)
		}
		if act == nil {
			t.Fatal("act is nil")
		}
		if act.Type != expectedCode {
			t.Fatalf("got action: %+v expected action code %c", act, expectedCode)
		}
	}
	assertContinue := func(act *Action, err error) {
		t.Helper()
		assert(act, err, ActionContinue)
	}
	assertEnd := func(mActions []ModifyAction, act *Action, err error) {
		t.Helper()
		assert(act, err, ActionAccept)
		if len(mActions) > 0 {
			t.Fatalf("milter returned ModifyActions: %+v", mActions)
		}
	}
	macros := NewMacroBag()
	w := newServerClient(t, macros, []Option{WithMilter(func() Milter {
		return NoOpMilter{}
	})}, nil)
	defer w.Cleanup()
	macros.Set(MacroMTAFQDN, "localhost.local")
	macros.Set(MacroTlsVersion, "TLS1.3")
	macros.Set(MacroAuthType, "plain")
	macros.Set(MacroRcptMailer, "smtp")
	macros.Set(MacroQueueId, "123")
	assertContinue(w.session.Conn("localhost", FamilyInet, 2525, "127.0.0.1"))
	assertContinue(w.session.Helo("localhost"))
	assertContinue(w.session.Mail("", ""))
	assertContinue(w.session.Rcpt("", ""))
	assertContinue(w.session.Rcpt("", ""))
	if err := w.session.Abort(nil); err != nil {
		t.Fatal(err)
	}
	assertContinue(w.session.Mail("", ""))
	assertContinue(w.session.Rcpt("", ""))
	assertContinue(w.session.Rcpt("", ""))
	hdrs := textproto.Header{}
	hdrs.Add("From", "Mailer Daemon <>")
	assertContinue(w.session.Header(hdrs))
	assertEnd(w.session.BodyReadFrom(bytes.NewReader([]byte("test\ntest\n"))))

	if err := w.session.Reset(nil); err != nil {
		t.Fatal(err)
	}

	assertContinue(w.session.Conn("localhost", FamilyInet, 2525, "127.0.0.1"))
	assertContinue(w.session.Helo("localhost"))
	assertContinue(w.session.Mail("", ""))
	assertContinue(w.session.Rcpt("", ""))
	assertContinue(w.session.DataStart())
	assertContinue(w.session.HeaderField("From", "<>", nil))
	assertContinue(w.session.HeaderField("To", "<>", nil))
	assertContinue(w.session.HeaderEnd())
	assertContinue(w.session.BodyChunk([]byte("test\n")))
	assertContinue(w.session.BodyChunk([]byte("test\n")))
	assertEnd(w.session.End())
	if err := w.server.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewServer(t *testing.T) {
	dummyMilter := func(version uint32, action OptAction, protocol OptProtocol, maxData DataSize) Milter {
		return &NoOpMilter{}
	}
	t.Parallel()
	tests := []struct {
		name      string
		opts      []Option
		check     func(s *Server) bool
		wantPanic bool
	}{
		{"milter missing", []Option{WithMaximumVersion(6)}, nil, true},
		{"with invalid version 1", []Option{WithDynamicMilter(dummyMilter), WithMaximumVersion(1)}, nil, true},
		{"with invalid version 10", []Option{WithDynamicMilter(dummyMilter), WithMaximumVersion(10)}, nil, true},
		{"with dialer", []Option{WithDynamicMilter(dummyMilter), WithDialer(&net.Dialer{})}, nil, true},
		{"with offered max data", []Option{WithDynamicMilter(dummyMilter), WithOfferedMaxData(123)}, nil, true},
		{"with macros", []Option{WithDynamicMilter(dummyMilter), WithMacroRequest(StageConnect, []MacroName{MacroClientName})}, func(s *Server) bool {
			return (s.options.actions & OptSetMacros) != 0
		}, false},
	}
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			defer func() { _ = recover() }()
			got := NewServer(tt.opts...)
			if tt.check != nil {
				if !tt.check(got) {
					t.Errorf("check failed, got %+v", got)
				}
			}
			if tt.wantPanic {
				t.Errorf("did not panic")
			}

		})
	}
}
