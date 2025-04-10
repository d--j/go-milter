package milter

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/emersion/go-message/textproto"
)

type mockModifier struct {
	version  uint32
	protocol OptProtocol
}

func (m *mockModifier) Get(name MacroName) string {
	return ""
}

func (m *mockModifier) GetEx(name MacroName) (value string, ok bool) {
	return "", false
}

func (m *mockModifier) Version() uint32 {
	return m.version
}

func (m *mockModifier) Protocol() OptProtocol {
	return m.protocol
}

func (m *mockModifier) Actions() OptAction {
	return AllClientSupportedActionMasks
}

func (m *mockModifier) MaxDataSize() DataSize {
	return DataSize64K
}

func (m *mockModifier) MilterId() uint64 {
	return 0
}

func (m *mockModifier) AddRecipient(r string, esmtpArgs string) error {
	panic("not implemented")
}

func (m *mockModifier) DeleteRecipient(r string) error {
	panic("not implemented")
}

func (m *mockModifier) ReplaceBodyRawChunk(chunk []byte) error {
	panic("not implemented")
}

func (m *mockModifier) ReplaceBody(r io.Reader) error {
	panic("not implemented")
}

func (m *mockModifier) Quarantine(reason string) error {
	panic("not implemented")
}

func (m *mockModifier) AddHeader(name, value string) error {
	panic("not implemented")
}

func (m *mockModifier) ChangeHeader(index int, name, value string) error {
	panic("not implemented")
}

func (m *mockModifier) InsertHeader(index int, name, value string) error {
	panic("not implemented")
}

func (m *mockModifier) ChangeFrom(value string, esmtpArgs string) error {
	panic("not implemented")
}

func (m *mockModifier) Progress() error {
	panic("not implemented")
}

var _ Modifier = (*mockModifier)(nil)

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
	mod := &mockModifier{version: 2, protocol: 0}
	assetContinue(m.Connect("", "", 0, "", mod))
	assetContinue(m.Helo("", mod))
	assetContinue(m.MailFrom("", "", mod))
	assetContinue(m.RcptTo("", "", mod))
	assetContinue(m.Unknown("", mod))
	assetContinue(m.Data(mod))
	assetContinue(m.Header("", "", mod))
	assetContinue(m.Headers(mod))
	assetContinue(m.BodyChunk(nil, mod))
	assetAccept(m.EndOfMessage(mod))
	m.Cleanup(mod)
}

func TestNoOpMilterV6(t *testing.T) {
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
	assetSkip := func(resp *Response, err error) {
		t.Helper()
		asset(resp, err, wire.ActSkip)
	}
	assetAccept := func(resp *Response, err error) {
		t.Helper()
		asset(resp, err, wire.ActAccept)
	}
	m := NoOpMilter{}
	mod := &mockModifier{version: 6, protocol: OptSkip}
	assetContinue(m.Connect("", "", 0, "", mod))
	assetContinue(m.Helo("", mod))
	assetContinue(m.MailFrom("", "", mod))
	assetSkip(m.RcptTo("", "", mod))
	assetContinue(m.Unknown("", mod))
	assetContinue(m.Data(mod))
	assetSkip(m.Header("", "", mod))
	assetContinue(m.Headers(mod))
	assetSkip(m.BodyChunk(nil, mod))
	assetAccept(m.EndOfMessage(mod))
	m.Cleanup(mod)
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
	t.Cleanup(w.Cleanup)
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

func TestServer_Shutdown(t *testing.T) {
	t.Parallel()
	type args struct {
		mod func(wrap *serverClientWrap)
		ctx func() (context.Context, context.CancelFunc)
	}
	oneSecCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), time.Second)
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"active", args{func(wrap *serverClientWrap) {
			// newServerClient opens a session
		}, oneSecCtx}, true},
		{"idle", args{func(wrap *serverClientWrap) {
			_ = wrap.session.Close()
			time.Sleep(time.Millisecond * 100)
		}, oneSecCtx}, false},
		{"graceful", args{func(w *serverClientWrap) {
			go func() {
				if _, err := w.session.Conn("localhost", FamilyInet, 2525, "127.0.0.1"); err != nil {
					return
				}
				if _, err := w.session.Helo("localhost"); err != nil {
					return
				}
				if _, err := w.session.Mail("", ""); err != nil {
					return
				}
				if _, err := w.session.Rcpt("", ""); err != nil {
					return
				}
				if _, err := w.session.DataStart(); err != nil {
					return
				}
				if _, err := w.session.HeaderField("From", "<>", nil); err != nil {
					return
				}
				if _, err := w.session.HeaderField("To", "<>", nil); err != nil {
					return
				}
				if _, err := w.session.HeaderEnd(); err != nil {
					return
				}
				if _, err := w.session.BodyChunk([]byte("test\n")); err != nil {
					return
				}
				if _, _, err := w.session.End(); err != nil {
					return
				}
				if err := w.session.Close(); err != nil {
					return
				}
			}()
		}, oneSecCtx}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newServerClient(t, NewMacroBag(), []Option{WithMilter(func() Milter {
				return NoOpMilter{}
			})}, nil)
			t.Cleanup(w.Cleanup)
			tt.args.mod(&w)
			ctx, cancel := tt.args.ctx()
			defer cancel()
			if err := w.server.Shutdown(ctx); (err != nil) != tt.wantErr {
				t.Errorf("Shutdown() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type dummyDialer struct {
}

func (d *dummyDialer) Dial(network, address string) (net.Conn, error) {
	return nil, nil
}

func TestNewServerPanic(t *testing.T) {
	type args struct {
		opts []Option
	}
	tests := []struct {
		name string
		args args
	}{
		{"missing milter function", args{opts: []Option{WithDynamicMilter(nil)}}},
		{"wrong version", args{opts: []Option{WithMilter(nil), WithMaximumVersion(99)}}},
		{"with dialer", args{opts: []Option{WithMilter(nil), WithDialer(&dummyDialer{})}}},
		{"with offered max data", args{opts: []Option{WithMilter(nil), WithOfferedMaxData(DataSize1M)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("NewServer() did not panic")
				}
			}()
			NewServer(tt.args.opts...)
		})
	}
}

func TestServer_MilterCount(t *testing.T) {
	s := &Server{}
	s.milterCount.Store(1)
	if got := s.MilterCount(); got != 1 {
		t.Errorf("MilterCount() = %d, want %d", got, 1)
	}
}
