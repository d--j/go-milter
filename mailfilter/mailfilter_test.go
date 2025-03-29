package mailfilter

import (
	"context"
	"github.com/d--j/go-milter"
	"math"
	"net"
	"reflect"
	"testing"
	"time"
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
			endOfMessage,
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
			endOfMessage,
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
				if !reflect.DeepEqual(got.options, tt.want.options) {
					t.Errorf("New() resolvedOptions got = %+v, want %+v", got.options, tt.want.options)
				}
				if !reflect.DeepEqual(got.protocol, tt.want.protocol) {
					t.Errorf("New() protocol got = \n%032b\nwant\n%032b", got.protocol, tt.want.protocol)
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
