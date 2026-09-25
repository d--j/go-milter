package mailfilter

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d--j/go-milter"
)

type testListener struct {
	addr net.Addr
}

func (t testListener) Accept() (net.Conn, error) {
	panic("implement me")
}

func (t testListener) Close() error {
	panic("implement me")
}

func (t testListener) Addr() net.Addr {
	return t.addr
}

func TestMailFilter_Addr(t *testing.T) {
	testAddr := &net.TCPAddr{
		IP:   net.IP([]byte{127, 0, 0, 1}),
		Port: 1,
	}
	type fields struct {
		socket net.Listener
	}
	tests := []struct {
		name   string
		fields fields
		want   net.Addr
	}{
		{"nil", fields{nil}, nil},
		{"non-nil", fields{testListener{addr: testAddr}}, testAddr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &MailFilter{
				socket: tt.fields.socket,
			}
			if got := f.Addr(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Addr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	decider := func(_ context.Context, _ Trx) (Decision, error) {
		return Accept, nil
	}
	validatorAcceptAll := func(_ context.Context, _ *RcptToValidationInput) (Decision, error) {
		return Accept, nil
	}
	const endOfMessage = milter.OptHeaderLeadingSpace | milter.OptNoUnknown | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoRcptReply | milter.OptNoHeaderReply | milter.OptNoEOHReply | milter.OptNoBodyReply
	type args struct {
		network  string
		address  string
		decision DecisionModificationFunc
		opts     []Option
	}
	type want struct {
		options  options
		protocol milter.OptProtocol
	}
	tests := []struct {
		name    string
		args    args
		want    want
		wantErr bool
	}{
		{"err", args{}, want{options{}, 0}, true},
		{"listen-err", args{"tcp", "bogus", decider, nil}, want{options{}, 0}, true},
		{"defaults", args{"tcp", "127.0.0.1:", decider, nil}, want{
			options{
				decisionAt:    DecisionAtEndOfMessage,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			endOfMessage,
		}, false},
		{"decision-at-connect", args{"tcp", "127.0.0.1:", decider, []Option{WithDecisionAt(DecisionAtConnect)}}, want{
			options{
				decisionAt:    DecisionAtConnect,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			milter.OptHeaderLeadingSpace | milter.OptNoUnknown | milter.OptNoHelo | milter.OptNoMailFrom | milter.OptNoRcptTo | milter.OptNoData | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody,
		}, false},
		{"decision-at-helo", args{"tcp", "127.0.0.1:", decider, []Option{WithDecisionAt(DecisionAtHelo)}}, want{
			options{
				decisionAt:    DecisionAtHelo,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			milter.OptHeaderLeadingSpace | milter.OptNoUnknown | milter.OptNoConnReply | milter.OptNoMailFrom | milter.OptNoRcptTo | milter.OptNoData | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody,
		}, false},
		{"decision-at-mail-from", args{"tcp", "127.0.0.1:", decider, []Option{WithDecisionAt(DecisionAtMailFrom)}}, want{
			options{
				decisionAt:    DecisionAtMailFrom,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			milter.OptHeaderLeadingSpace | milter.OptNoUnknown | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoRcptTo | milter.OptNoData | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody,
		}, false},
		{"decision-at-data", args{"tcp", "127.0.0.1:", decider, []Option{WithDecisionAt(DecisionAtData)}}, want{
			options{
				decisionAt:    DecisionAtData,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			milter.OptHeaderLeadingSpace | milter.OptNoUnknown | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoMailReply | milter.OptNoRcptReply | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody,
		}, false},
		{"decision-at-eoh", args{"tcp", "127.0.0.1:", decider, []Option{WithDecisionAt(DecisionAtEndOfHeaders)}}, want{
			options{
				decisionAt:    DecisionAtEndOfHeaders,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			milter.OptHeaderLeadingSpace | milter.OptNoUnknown | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoMailReply | milter.OptNoRcptReply | milter.OptNoDataReply | milter.OptNoHeaderReply | milter.OptNoBody,
		}, false},
		{"without-body", args{"tcp", "127.0.0.1:", decider, []Option{WithoutBody()}}, want{
			options{
				decisionAt:    DecisionAtEndOfMessage,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip: true,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			endOfMessage | milter.OptNoBody,
		}, false},
		{"with-body", args{"tcp", "127.0.0.1:", decider, []Option{WithBody(12, 34, RejectMessageWhenTooBig)}}, want{
			options{
				decisionAt:    DecisionAtEndOfMessage,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    12,
					MaxSize:   34,
					MaxAction: RejectMessageWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			endOfMessage & ^milter.OptNoBodyReply,
		}, false},
		{"with-body-err-1", args{"tcp", "127.0.0.1:", decider, []Option{WithBody(12, 34, 99)}}, want{}, true},
		{"with-body-err-2", args{"tcp", "127.0.0.1:", decider, []Option{WithBody(12, 0, RejectMessageWhenTooBig)}}, want{}, true},
		{"with-body-err-3", args{"tcp", "127.0.0.1:", decider, []Option{WithBody(-12, 34, RejectMessageWhenTooBig)}}, want{}, true},
		{"with-header", args{"tcp", "127.0.0.1:", decider, []Option{WithHeader(1, RejectMessageWhenTooBig)}}, want{
			options{
				decisionAt:    DecisionAtEndOfMessage,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       1,
					MaxAction: RejectMessageWhenTooBig,
				},
			},
			endOfMessage & ^milter.OptNoHeaderReply,
		}, false},
		{"with-header-big", args{"tcp", "127.0.0.1:", decider, []Option{WithHeader(math.MaxUint32, TruncateWhenTooBig)}}, want{
			options{
				decisionAt:    DecisionAtEndOfMessage,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       math.MaxUint32,
					MaxAction: TruncateWhenTooBig,
				},
			},
			endOfMessage,
		}, false},
		{"with-header-err-1", args{"tcp", "127.0.0.1:", decider, []Option{WithHeader(0, RejectMessageWhenTooBig)}}, want{}, true},
		{"with-header-err-2", args{"tcp", "127.0.0.1:", decider, []Option{WithHeader(1, 99)}}, want{}, true},
		{"error-handling", args{"tcp", "127.0.0.1:", decider, []Option{WithErrorHandling(RejectWhenError)}}, want{
			options{
				decisionAt:    DecisionAtEndOfMessage,
				errorHandling: RejectWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
			},
			endOfMessage,
		}, false},
		{"error-handling-err", args{"tcp", "127.0.0.1:", decider, []Option{WithErrorHandling(99)}}, want{}, true},
		{"rcpt-to-validator", args{"tcp", "127.0.0.1:", decider, []Option{WithRcptToValidator(validatorAcceptAll), WithDecisionAt(DecisionAtData)}}, want{
			options{
				decisionAt:    DecisionAtData,
				errorHandling: TempFailWhenError,
				body: &bodyOption{
					Skip:      false,
					MaxMem:    1024 * 200,
					MaxSize:   1024 * 1024 * 100,
					MaxAction: TruncateWhenTooBig,
				},
				header: &headerOption{
					Max:       512,
					MaxAction: TruncateWhenTooBig,
				},
				rcptToValidator: validatorAcceptAll,
			},
			milter.OptHeaderLeadingSpace | milter.OptNoUnknown | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoMailReply | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody,
		}, false},
		{"decisionAt-err", args{"tcp", "127.0.0.1:", decider, []Option{WithDecisionAt(99)}}, want{}, true},
		{"rcptToValidator-err", args{"tcp", "127.0.0.1:", decider, []Option{WithRcptToValidator(validatorAcceptAll), WithDecisionAt(DecisionAtConnect)}}, want{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.network, tt.args.address, tt.args.decision, tt.args.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got == nil {
					t.Fatal("New() is nil")
				}
				t.Cleanup(got.Close)
				if (got.options.rcptToValidator == nil) != (tt.want.options.rcptToValidator == nil) {
					t.Errorf("New() resolvedOptions.rcptToValidator got = %+v, want %+v", got.options.rcptToValidator, tt.want.options.rcptToValidator)
				}
				got.options.rcptToValidator = nil
				tt.want.options.rcptToValidator = nil
				if !reflect.DeepEqual(got.options, tt.want.options) {
					t.Errorf("New() resolvedOptions got = %+v, want %+v", got.options, tt.want.options)
				}
				if !reflect.DeepEqual(got.protocol, tt.want.protocol) {
					t.Errorf("New() protocol got = \n%q\nwant\n%q", got.protocol, tt.want.protocol)
				}
				waited := make(chan struct{}, 1)
				go func() {
					got.Wait()
					waited <- struct{}{}
				}()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*2)
				defer cancel()
				_ = got.Shutdown(shutdownCtx)
				select {
				case <-waited:
				case <-time.After(5 * time.Second):
					t.Fatalf("Wait() timeout")
				}
			}
		})
	}
}

func TestMailFilter_DecisionPanic(t *testing.T) {
	shouldPanic := true
	warningChan := make(chan string, 10)
	origLogWarning := milter.LogWarning
	milter.LogWarning = func(format string, v ...any) {
		warningChan <- fmt.Sprintf(format, v...)
	}
	defer func() {
		milter.LogWarning = origLogWarning
	}()

	filter, err := New("tcp", "127.0.0.1:0", func(ctx context.Context, trx Trx) (Decision, error) {
		if shouldPanic {
			panic("panic in decision")
		}
		return Accept, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer filter.Close()

	client := milter.NewClient("tcp", filter.Addr().String())

	// First session panics in decision
	session, err := client.Session(nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = func() error {
		if _, err := session.Conn("localhost", milter.FamilyInet, 2525, "127.0.0.1"); err != nil {
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
	}()

	var warningMsg string
	select {
	case warningMsg = <-warningChan:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for panic warning log")
	}

	if !strings.Contains(warningMsg, "panic in milter session") {
		t.Fatalf("expected panic warning log, got %q", warningMsg)
	}

	// Second session on same server succeeds
	shouldPanic = false
	session2, err := client.Session(nil)
	if err != nil {
		t.Fatalf("session2 connect failed: %v", err)
	}
	if _, err := session2.Conn("localhost", milter.FamilyInet, 2525, "127.0.0.1"); err != nil {
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := filter.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("filter.Shutdown failed: %v", err)
	}
}

func TestWithOnPanic(t *testing.T) {
	opt := options{}
	var got any
	WithOnPanic(func(v any) {
		got = v
	})(&opt)
	if opt.onPanicCallback == nil {
		t.Fatalf("did not set onPanicCallback")
	}
	opt.onPanicCallback("boom")
	if got != "boom" {
		t.Fatalf("did not set the correct onPanicCallback, got %v", got)
	}
}

// runPanicSession runs a full SMTP transaction against addr and ignores all errors
// (the server is expected to drop the connection when it panics).
func runPanicSession(addr string) {
	client := milter.NewClient("tcp", addr)
	session, err := client.Session(nil)
	if err != nil {
		return
	}
	defer session.Close()
	if _, err := session.Conn("localhost", milter.FamilyInet, 2525, "127.0.0.1"); err != nil {
		return
	}
	if _, err := session.Helo("localhost"); err != nil {
		return
	}
	if _, err := session.Mail("sender@example.com", ""); err != nil {
		return
	}
	if _, err := session.Rcpt("rcpt@example.com", ""); err != nil {
		return
	}
	if _, err := session.DataStart(); err != nil {
		return
	}
	if _, err := session.HeaderField("Subject", "Test", nil); err != nil {
		return
	}
	if _, err := session.HeaderEnd(); err != nil {
		return
	}
	_, _, _ = session.BodyReadFrom(bytes.NewReader([]byte("test\n")))
}

func TestMailFilter_OnPanic(t *testing.T) {
	tests := []struct {
		name      string
		decision  DecisionModificationFunc
		opts      []Option
		wantPanic string
	}{
		{
			name: "decision",
			decision: func(ctx context.Context, trx Trx) (Decision, error) {
				panic("panic in decision")
			},
			wantPanic: "panic in decision",
		},
		{
			name: "rcptToValidator",
			decision: func(ctx context.Context, trx Trx) (Decision, error) {
				return Accept, nil
			},
			opts: []Option{WithRcptToValidator(func(ctx context.Context, in *RcptToValidationInput) (Decision, error) {
				panic("panic in validator")
			})},
			wantPanic: "panic in validator",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warningChan := make(chan string, 10)
			origLogWarning := milter.LogWarning
			milter.LogWarning = func(format string, v ...any) {
				warningChan <- fmt.Sprintf(format, v...)
			}
			defer func() {
				milter.LogWarning = origLogWarning
			}()

			panicChan := make(chan any, 1)
			opts := append(tt.opts, WithOnPanic(func(v any) {
				panicChan <- v
			}))
			filter, err := New("tcp", "127.0.0.1:0", tt.decision, opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer filter.Close()

			runPanicSession(filter.Addr().String())

			select {
			case v := <-panicChan:
				if v != tt.wantPanic {
					t.Fatalf("onPanicCallback got %v, want %q", v, tt.wantPanic)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for onPanicCallback")
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := filter.Shutdown(shutdownCtx); err != nil {
				t.Fatalf("filter.Shutdown failed: %v", err)
			}

			milter.LogWarning = origLogWarning
			close(warningChan)
			for msg := range warningChan {
				if strings.Contains(msg, "panic") {
					t.Fatalf("LogWarning should not be called when WithOnPanic is set, got %q", msg)
				}
			}
		})
	}
}

func TestMailFilter_RcptToValidatorPanicWithoutCallback(t *testing.T) {
	warningChan := make(chan string, 10)
	origLogWarning := milter.LogWarning
	milter.LogWarning = func(format string, v ...any) {
		warningChan <- fmt.Sprintf(format, v...)
	}
	defer func() {
		milter.LogWarning = origLogWarning
	}()

	filter, err := New("tcp", "127.0.0.1:0", func(ctx context.Context, trx Trx) (Decision, error) {
		return Accept, nil
	}, WithRcptToValidator(func(ctx context.Context, in *RcptToValidationInput) (Decision, error) {
		panic("panic in validator")
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer filter.Close()

	runPanicSession(filter.Addr().String())

	select {
	case msg := <-warningChan:
		if !strings.Contains(msg, "panic in milter session") || !strings.Contains(msg, "panic in validator") {
			t.Fatalf("expected panic warning log, got %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for panic warning log")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := filter.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("filter.Shutdown failed: %v", err)
	}
}
