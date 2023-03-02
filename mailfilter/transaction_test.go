package mailfilter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/emersion/go-message/mail"
)

func rcptFromAddr(in []addr) []RcptTo {
	if in == nil {
		return nil
	}
	var out = []RcptTo{}
	for _, i := range in {
		out = append(out, RcptTo{addr: i})
	}
	return out
}
func addrFromRcp(in []RcptTo) []addr {
	if in == nil {
		return nil
	}
	var out = []addr{}
	for _, i := range in {
		out = append(out, addr{Addr: i.addr.Addr, Args: i.addr.Args})
	}
	return out
}

func TestTransaction_AddRcptTo(t1 *testing.T) {
	type args struct {
		rcptTo    string
		esmtpArgs string
	}
	tests := []struct {
		name     string
		existing []addr
		args     args
		want     []addr
	}{
		{"nil", nil, args{"", ""}, []addr{{}}},
		{"empty", []addr{}, args{"", ""}, []addr{{}}},
		{"set-esmtp-args", []addr{{Args: ""}}, args{"", "A=B"}, []addr{{Args: "A=B"}}},
		{"add", []addr{{}}, args{"root@localhost", "A=B"}, []addr{{}, {Addr: "root@localhost", Args: "A=B"}}},
		{"idna-utf8", []addr{{Addr: "root@スパム.example.com"}}, args{"root@xn--zck5b2b.example.com", "A=B"}, []addr{{Addr: "root@スパム.example.com", Args: "A=B"}}},
		{"idna-ascii", []addr{{Addr: "root@xn--zck5b2b.example.com"}}, args{"root@スパム.example.com", "A=B"}, []addr{{Addr: "root@xn--zck5b2b.example.com", Args: "A=B"}}},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {

			t := &Transaction{
				RcptTos: rcptFromAddr(tt.existing),
			}
			t.AddRcptTo(tt.args.rcptTo, tt.args.esmtpArgs)
			got := addrFromRcp(t.RcptTos)
			if !reflect.DeepEqual(got, tt.want) {
				t1.Fatalf("RcptTos = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTransaction_DelRcptTo(t1 *testing.T) {
	type args struct {
		rcptTo string
	}
	tests := []struct {
		name     string
		existing []addr
		args     args
		want     []addr
	}{
		{"nil", nil, args{""}, nil},
		{"empty", []addr{}, args{""}, []addr{}},
		{"del", []addr{{Addr: "root@localhost"}}, args{"root@localhost"}, []addr{}},
		{"idna-utf8", []addr{{Addr: "root@スパム.example.com"}}, args{"root@xn--zck5b2b.example.com"}, []addr{}},
		{"idna-ascii", []addr{{Addr: "root@xn--zck5b2b.example.com"}}, args{"root@スパム.example.com"}, []addr{}},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transaction{
				RcptTos: rcptFromAddr(tt.existing),
			}
			t.DelRcptTo(tt.args.rcptTo)
			got := addrFromRcp(t.RcptTos)
			if !reflect.DeepEqual(got, tt.want) {
				t1.Fatalf("RcptTos = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransaction_HasRcptTo(t1 *testing.T) {
	type args struct {
		rcptTo string
	}
	tests := []struct {
		name     string
		existing []addr
		args     args
		want     bool
	}{
		{"nil", nil, args{""}, false},
		{"empty", []addr{}, args{""}, false},
		{"no", []addr{{Addr: "root@localhost"}}, args{""}, false},
		{"yes", []addr{{Addr: "root@localhost"}}, args{"root@localhost"}, true},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transaction{
				RcptTos: rcptFromAddr(tt.existing),
			}
			if got := t.HasRcptTo(tt.args.rcptTo); got != tt.want {
				t1.Errorf("HasRcptTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func outputMessages(messages []*wire.Message) string {
	b := strings.Builder{}
	for i, msg := range messages {
		b.WriteString(fmt.Sprintf("%02d %c %q\n", i, msg.Code, msg.Data))
	}
	return b.String()
}

func TestTransaction_sendModifications(t1 *testing.T) {
	expectErr := errors.New("error")
	writeErr := func(_ *wire.Message) error {
		return expectErr
	}
	mod := func(act wire.ModifyActCode, data []byte) *wire.Message {
		return &wire.Message{Code: wire.Code(act), Data: data}
	}
	tests := []struct {
		name    string
		decider DecisionModificationFunc
		want    []*wire.Message
		wantErr bool
	}{
		{"noop", func(_ context.Context, trx *Transaction) (Decision, error) {
			return Accept, nil
		}, nil, false},
		{"mail-from", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.MailFrom.Addr = "root@localhost"
			trx.MailFrom.Args = "A=B"
			return Accept, nil
		}, []*wire.Message{mod(wire.ActChangeFrom, []byte("<root@localhost>\u0000A=B\u0000"))}, false},
		{"mail-from-err", func(ctx context.Context, trx *Transaction) (Decision, error) {
			trx.MailFrom.Addr = "root@localhost"
			ctx.Value("s").(*mockSession).WritePacket = writeErr
			return Accept, nil
		}, nil, true},
		{"del-rcpt", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.DelRcptTo("root@localhost")
			return Accept, nil
		}, []*wire.Message{mod(wire.ActDelRcpt, []byte("<root@localhost>\u0000"))}, false},
		{"del-rcpt-noop", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.DelRcptTo("someone@localhost")
			return Accept, nil
		}, nil, false},
		{"del-rcpt-err", func(ctx context.Context, trx *Transaction) (Decision, error) {
			trx.DelRcptTo("root@localhost")
			ctx.Value("s").(*mockSession).WritePacket = writeErr
			return Accept, nil
		}, nil, true},
		{"add-rcpt", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.AddRcptTo("someone@localhost", "")
			return Accept, nil
		}, []*wire.Message{mod(wire.ActAddRcpt, []byte("<someone@localhost>\u0000"))}, false},
		{"add-rcpt-par", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.AddRcptTo("someone@localhost", "A=B")
			return Accept, nil
		}, []*wire.Message{mod(wire.ActAddRcptPar, []byte("<someone@localhost>\u0000A=B\u0000"))}, false},
		{"add-rcpt-noop", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.AddRcptTo("root@localhost", "")
			return Accept, nil
		}, nil, false},
		{"add-rcpt-err", func(ctx context.Context, trx *Transaction) (Decision, error) {
			trx.AddRcptTo("someone@localhost", "")
			ctx.Value("s").(*mockSession).WritePacket = writeErr
			return Accept, nil
		}, nil, true},
		{"replace-rcpt", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.RcptTos[0].Addr = "someone@localhost"
			return Accept, nil
		}, []*wire.Message{
			mod(wire.ActDelRcpt, []byte("<root@localhost>\u0000")),
			mod(wire.ActAddRcpt, []byte("<someone@localhost>\u0000")),
		}, false},
		{"replace-body", func(_ context.Context, trx *Transaction) (Decision, error) {
			got, _ := io.ReadAll(trx.Body())
			if string(got) != "body" {
				t1.Fatalf("wrong body %q", got)
			}
			trx.ReplaceBody(io.NopCloser(strings.NewReader("test")))
			return Accept, nil
		}, []*wire.Message{
			mod(wire.ActReplBody, []byte("test")),
		}, false},
		{"replace-body-err", func(ctx context.Context, trx *Transaction) (Decision, error) {
			trx.ReplaceBody(io.NopCloser(strings.NewReader("test")))
			ctx.Value("s").(*mockSession).WritePacket = writeErr
			return Accept, nil
		}, nil, true},
		{"add-header", func(_ context.Context, trx *Transaction) (Decision, error) {
			trx.Headers.Add("X-Test", "1")
			return Accept, nil
		}, []*wire.Message{
			mod(wire.ActInsertHeader, []byte("\u0000\u0000\u0000\u0003X-Test\u0000 1\u0000")),
		}, false},
		{"prepend-header", func(_ context.Context, trx *Transaction) (Decision, error) {
			f := trx.Headers.Fields()
			f.Next()
			f.InsertBefore("X-Test", "1")
			return Accept, nil
		}, []*wire.Message{
			mod(wire.ActInsertHeader, []byte("\u0000\u0000\u0000\u0000X-Test\u0000 1\u0000")),
		}, false},
		{"prepend-header-err", func(ctx context.Context, trx *Transaction) (Decision, error) {
			f := trx.Headers.Fields()
			f.Next()
			f.InsertBefore("X-Test", "1")
			ctx.Value("s").(*mockSession).WritePacket = writeErr
			return Accept, nil
		}, nil, true},
		{"change-header", func(_ context.Context, trx *Transaction) (Decision, error) {
			f := trx.Headers.Fields()
			f.Next()
			f.SetAddressList([]*mail.Address{{Address: "root@localhost", Name: "root"}})
			return Accept, nil
		}, []*wire.Message{
			mod(wire.ActChangeHeader, []byte("\u0000\u0000\u0000\u0001From\u0000 \"root\" <root@localhost>\u0000")),
		}, false},
		{"change-header-err", func(ctx context.Context, trx *Transaction) (Decision, error) {
			f := trx.Headers.Fields()
			f.Next()
			f.SetAddressList([]*mail.Address{{Address: "root@localhost", Name: "root"}})
			ctx.Value("s").(*mockSession).WritePacket = writeErr
			return Accept, nil
		}, nil, true},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			b, s := newMockBackend()
			t1.Cleanup(b.transaction.cleanup)
			_, _ = b.MailFrom("", "", s.newModifier())
			_, _ = b.RcptTo("root@localhost", "", s.newModifier())
			_, _ = b.Header("From", " <>", s.newModifier())
			_, _ = b.Header("To", " <root@localhost>", s.newModifier())
			_, _ = b.Header("Subject", " test", s.newModifier())
			_, _ = b.BodyChunk([]byte("body"), s.newModifier())
			b.transaction.makeDecision(context.WithValue(context.Background(), "s", s), tt.decider)
			if b.transaction.decisionErr != nil {
				t1.Fatal(b.transaction.decisionErr)
			}
			if tt.wantErr == false {
				gotHas := b.transaction.hasModifications()
				expectHas := false
				if len(tt.want) > 0 {
					expectHas = true
				}
				if gotHas != expectHas {
					t1.Errorf("hasModifications() = %v, want %v", gotHas, expectHas)
				}
			}
			if err := b.transaction.sendModifications(s.newModifier()); (err != nil) != tt.wantErr {
				t1.Errorf("sendModifications() error = %v, wantErr %v", err, tt.wantErr)
			}
			got := s.modifications
			if !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("sendModifications() sent %v, want %v", outputMessages(got), outputMessages(tt.want))
			}
		})
	}
}
