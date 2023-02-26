package milter

import (
	"bytes"
	"testing"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/emersion/go-message/textproto"
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
	macros.Set(MacroMTAFullyQualifiedDomainName, "localhost.local")
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
