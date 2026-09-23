package milter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
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

type stagePanicMilter struct {
	NoOpMilter
	panicAt string
}

func (m *stagePanicMilter) NewConnection(mod Modifier) error {
	if m.panicAt == "NewConnection" {
		panic("panic in NewConnection")
	}
	return nil
}

func (m *stagePanicMilter) Connect(host string, family string, port uint16, addr string, mod Modifier) (*Response, error) {
	if m.panicAt == "Connect" {
		panic("panic in Connect")
	}
	return RespContinue, nil
}

func (m *stagePanicMilter) Helo(name string, mod Modifier) (*Response, error) {
	if m.panicAt == "Helo" {
		panic("panic in Helo")
	}
	return RespContinue, nil
}

func (m *stagePanicMilter) MailFrom(from string, esmtpArgs string, mod Modifier) (*Response, error) {
	if m.panicAt == "MailFrom" {
		panic("panic in MailFrom")
	}
	return RespContinue, nil
}

func (m *stagePanicMilter) RcptTo(rcptTo string, esmtpArgs string, mod Modifier) (*Response, error) {
	if m.panicAt == "RcptTo" {
		panic("panic in RcptTo")
	}
	return RespContinue, nil
}

func (m *stagePanicMilter) Header(name string, value string, mod Modifier) (*Response, error) {
	if m.panicAt == "Header" {
		panic("panic in Header")
	}
	return RespContinue, nil
}

func (m *stagePanicMilter) Headers(mod Modifier) (*Response, error) {
	if m.panicAt == "Headers" {
		panic("panic in Headers")
	}
	return RespContinue, nil
}

func (m *stagePanicMilter) BodyChunk(chunk []byte, mod Modifier) (*Response, error) {
	if m.panicAt == "BodyChunk" {
		panic("panic in BodyChunk")
	}
	return RespContinue, nil
}

func (m *stagePanicMilter) EndOfMessage(mod Modifier) (*Response, error) {
	if m.panicAt == "EndOfMessage" {
		panic("panic in EndOfMessage")
	}
	return RespAccept, nil
}

func (m *stagePanicMilter) Abort(mod Modifier) error {
	if m.panicAt == "Abort" {
		panic("panic in Abort")
	}
	return nil
}

func (m *stagePanicMilter) Cleanup(mod Modifier) {
	if m.panicAt == "Cleanup" {
		panic("panic in Cleanup")
	}
}

func TestServer_PanicRecovery(t *testing.T) {
	stages := []string{
		"newMilter",
		"NewConnection",
		"Connect",
		"Helo",
		"MailFrom",
		"RcptTo",
		"Header",
		"Headers",
		"BodyChunk",
		"EndOfMessage",
		"Abort",
		"Cleanup",
	}

	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			currentPanic := stage
			warningChan := make(chan string, 10)
			origLogWarning := LogWarning
			LogWarning = func(format string, v ...any) {
				warningChan <- fmt.Sprintf(format, v...)
			}
			defer func() {
				LogWarning = origLogWarning
			}()

			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()

			server := NewServer(
				WithDynamicMilter(func(version uint32, action OptAction, protocol OptProtocol, maxData DataSize) Milter {
					if currentPanic == "newMilter" {
						panic("panic in newMilter")
					}
					return &stagePanicMilter{panicAt: currentPanic}
				}),
			)

			go func() {
				_ = server.Serve(ln)
			}()
			defer server.Close()

			client := NewClient("tcp", ln.Addr().String())
			session, err := client.Session(nil)
			if err == nil {
				runSession := func() error {
					if _, err := session.Conn("localhost", FamilyInet, 2525, "127.0.0.1"); err != nil {
						return err
					}
					if _, err := session.Helo("localhost"); err != nil {
						return err
					}
					if _, err := session.Mail("sender@example.com", ""); err != nil {
						return err
					}
					if _, err := session.Rcpt("rcpt@example.com", ""); err != nil {
						return err
					}
					if stage == "Abort" {
						return session.Abort(nil)
					}
					if _, err := session.DataStart(); err != nil {
						return err
					}
					if _, err := session.HeaderField("Subject", "Test", nil); err != nil {
						return err
					}
					if _, err := session.HeaderEnd(); err != nil {
						return err
					}
					if _, _, err := session.BodyReadFrom(bytes.NewReader([]byte("test\n"))); err != nil {
						return err
					}
					return session.Close()
				}
				_ = runSession()
			}

			var warningMsg string
			select {
			case warningMsg = <-warningChan:
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for panic warning log")
			}

			if !strings.Contains(warningMsg, "panic in milter session") {
				t.Fatalf("expected panic warning log, got %q", warningMsg)
			}

			// Verify server is still functional: new session succeeds
			currentPanic = ""
			session2, err := client.Session(nil)
			if err != nil {
				t.Fatalf("new session after panic failed to connect: %v", err)
			}
			if _, err := session2.Conn("localhost", FamilyInet, 2525, "127.0.0.1"); err != nil {
				t.Fatalf("session2 Conn failed: %v", err)
			}
			if _, err := session2.Helo("localhost"); err != nil {
				t.Fatalf("session2 Helo failed: %v", err)
			}
			if _, err := session2.Mail("sender@example.com", ""); err != nil {
				t.Fatalf("session2 Mail failed: %v", err)
			}
			if _, err := session2.Rcpt("rcpt@example.com", ""); err != nil {
				t.Fatalf("session2 Rcpt failed: %v", err)
			}
			if _, err := session2.DataStart(); err != nil {
				t.Fatalf("session2 DataStart failed: %v", err)
			}
			if _, err := session2.HeaderField("Subject", "Test", nil); err != nil {
				t.Fatalf("session2 HeaderField failed: %v", err)
			}
			if _, err := session2.HeaderEnd(); err != nil {
				t.Fatalf("session2 HeaderEnd failed: %v", err)
			}
			if _, _, err := session2.BodyReadFrom(bytes.NewReader([]byte("test\n"))); err != nil {
				t.Fatalf("session2 BodyReadFrom failed: %v", err)
			}
			if err := session2.Close(); err != nil {
				t.Fatalf("session2 Close failed: %v", err)
			}

			// Verify shutdown succeeds gracefully
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				t.Fatalf("server.Shutdown failed after recovered panic: %v", err)
			}
		})
	}
}
