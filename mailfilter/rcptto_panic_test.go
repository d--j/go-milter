package mailfilter

import (
	"context"
	"errors"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/d--j/go-milter"
)

func TestMailFilter_RcptToValidatorPanic(t *testing.T) {
	filter, err := New("tcp", "127.0.0.1:0", func(context.Context, Trx) (Decision, error) {
		return Accept, nil
	}, WithDecisionAt(DecisionAtData), WithRcptToValidator(func(_ context.Context, in *RcptToValidationInput) (Decision, error) {
		if in.RcptTo.Addr == "panic@example.com" {
			panic("panic in recipient validator")
		}
		return Accept, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer filter.Close()

	client := milter.NewClient("tcp", filter.Addr().String(), milter.WithReadTimeout(2*time.Second))
	newSession := func() *milter.ClientSession {
		t.Helper()
		session, err := client.Session(nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		if _, err := session.Conn("localhost", milter.FamilyInet, 2525, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
		if _, err := session.Helo("localhost"); err != nil {
			t.Fatal(err)
		}
		if _, err := session.Mail("sender@example.com", ""); err != nil {
			t.Fatal(err)
		}
		return session
	}

	// The panic must close this session, rather than accept the recipient or hang.
	session := newSession()
	if _, err := session.Rcpt("panic@example.com", ""); !errors.Is(err, io.EOF) {
		t.Fatalf("Rcpt() error = %v, want EOF after validator panic", err)
	}

	// A new session on the same server must still be able to filter mail.
	session = newSession()
	act, err := session.Rcpt("recipient@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if act.Type != milter.ActionContinue {
		t.Fatalf("Rcpt() action = %v, want continue", act.Type)
	}
	act, err = session.DataStart()
	if err != nil {
		t.Fatal(err)
	}
	if act.Type != milter.ActionAccept {
		t.Fatalf("DataStart() action = %v, want accept", act.Type)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := filter.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() after validator panic: %v", err)
	}
}

func Test_backend_RcptToValidatorPanic(t *testing.T) {
	for _, waitForCancellation := range []bool{false, true} {
		name := "immediate"
		if waitForCancellation {
			name = "after progress error"
		}
		t.Run(name, func(t *testing.T) {
			b, s := newMockBackend()
			s.progressErr = io.ErrClosedPipe
			wantPanic := errors.New("validator panic")
			var callbackCtx context.Context
			b.opts.rcptToValidator = func(ctx context.Context, _ *RcptToValidationInput) (Decision, error) {
				callbackCtx = ctx
				if waitForCancellation {
					<-ctx.Done()
				}
				panic(wantPanic)
			}
			defer func() {
				if got := recover(); got != wantPanic {
					t.Errorf("RcptTo() panic = %v, want %v", got, wantPanic)
				}
				if callbackCtx == nil || callbackCtx.Err() != context.Canceled {
					t.Error("validator context was not canceled")
				}
				if len(b.transaction.origRcptTos) != 0 {
					t.Error("recipient was accepted after validator panic")
				}
			}()
			_, _ = b.RcptTo("recipient@example.com", "", s.mod)
			t.Error("RcptTo() returned instead of propagating the validator panic")
		})
	}
}

func Test_backend_RcptToValidatorGoexit(t *testing.T) {
	b, s := newMockBackend()
	b.opts.rcptToValidator = func(context.Context, *RcptToValidationInput) (Decision, error) {
		runtime.Goexit()
		return Accept, nil
	}
	if _, err := b.RcptTo("recipient@example.com", "", s.mod); err == nil {
		t.Error("RcptTo() error = nil, want error when the validator does not return")
	}
	if len(b.transaction.origRcptTos) != 0 {
		t.Error("recipient was accepted after the validator exited without returning")
	}
}
