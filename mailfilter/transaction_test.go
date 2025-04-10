package mailfilter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/internal/body"
	"github.com/d--j/go-milter/internal/header"
	"github.com/d--j/go-milter/mailfilter/addr"
	"github.com/emersion/go-message/mail"
)

type a struct {
	Addr, Args string
}

func rcptFromAddr(in []a) []*addr.RcptTo {
	if in == nil {
		return nil
	}
	var out []*addr.RcptTo
	for _, i := range in {
		out = append(out, addr.NewRcptTo(i.Addr, i.Args, ""))
	}
	return out
}
func addrFromRcp(in []*addr.RcptTo) []a {
	if in == nil {
		return nil
	}
	var out []a
	for _, i := range in {
		out = append(out, a{Addr: i.Addr, Args: i.Args})
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
		existing []a
		args     args
		want     []a
	}{
		{"nil", nil, args{"", ""}, []a{{}}},
		{"empty", []a{}, args{"", ""}, []a{{}}},
		{"set-esmtp-args", []a{{Args: ""}}, args{"", "A=B"}, []a{{Args: "A=B"}}},
		{"add", []a{{}}, args{"root@localhost", "A=B"}, []a{{}, {Addr: "root@localhost", Args: "A=B"}}},
		{"idna-utf8", []a{{Addr: "root@スパム.example.com"}}, args{"root@xn--zck5b2b.example.com", "A=B"}, []a{{Addr: "root@スパム.example.com", Args: "A=B"}}},
		{"idna-ascii", []a{{Addr: "root@xn--zck5b2b.example.com"}}, args{"root@スパム.example.com", "A=B"}, []a{{Addr: "root@xn--zck5b2b.example.com", Args: "A=B"}}},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {

			t := &transaction{
				rcptTos: rcptFromAddr(tt.existing),
			}
			t.AddRcptTo(tt.args.rcptTo, tt.args.esmtpArgs)
			got := addrFromRcp(t.RcptTos())
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
		existing []a
		args     args
		want     []a
	}{
		{"nil", nil, args{""}, nil},
		{"empty", []a{}, args{""}, nil},
		{"del", []a{{Addr: "root@localhost"}}, args{"root@localhost"}, nil},
		{"idna-utf8", []a{{Addr: "root@スパム.example.com"}}, args{"root@xn--zck5b2b.example.com"}, nil},
		{"idna-ascii", []a{{Addr: "root@xn--zck5b2b.example.com"}}, args{"root@スパム.example.com"}, nil},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &transaction{
				rcptTos: rcptFromAddr(tt.existing),
			}
			t.DelRcptTo(tt.args.rcptTo)
			got := addrFromRcp(t.RcptTos())
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
		existing []a
		args     args
		want     bool
	}{
		{"nil", nil, args{""}, false},
		{"empty", []a{}, args{""}, false},
		{"no", []a{{Addr: "root@localhost"}}, args{""}, false},
		{"yes", []a{{Addr: "root@localhost"}}, args{"root@localhost"}, true},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &transaction{
				rcptTos: rcptFromAddr(tt.existing),
			}
			if got := t.HasRcptTo(tt.args.rcptTo); got != tt.want {
				t1.Errorf("HasRcptTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func outputMessages(messages []*milter.ModifyAction) string {
	b := strings.Builder{}
	for i, msg := range messages {
		b.WriteString(fmt.Sprintf("%02d %s\n", i, msg))
	}
	return b.String()
}

func TestTransaction_sendModifications(t1 *testing.T) {
	expectErr := errors.New("error")
	tests := []struct {
		name    string
		decider DecisionModificationFunc
		want    []*milter.ModifyAction
		wantErr bool
	}{
		{"noop", func(_ context.Context, _ Trx) (Decision, error) {
			return Accept, nil
		}, nil, false},
		{"mail-from", func(_ context.Context, trx Trx) (Decision, error) {
			trx.ChangeMailFrom("root@localhost", "A=B")
			return Accept, nil
		}, []*milter.ModifyAction{{Type: milter.ActionChangeFrom, From: "root@localhost", FromArgs: "A=B"}}, false},
		{"mail-from-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			trx.ChangeMailFrom("root@localhost", "")
			return Accept, nil
		}, nil, true},
		{"del-rcpt", func(_ context.Context, trx Trx) (Decision, error) {
			trx.DelRcptTo("root@localhost")
			return Accept, nil
		}, []*milter.ModifyAction{{Type: milter.ActionDelRcpt, Rcpt: "root@localhost"}}, false},
		{"del-rcpt-noop", func(_ context.Context, trx Trx) (Decision, error) {
			trx.DelRcptTo("someone@localhost")
			return Accept, nil
		}, nil, false},
		{"del-rcpt-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			trx.DelRcptTo("root@localhost")
			return Accept, nil
		}, nil, true},
		{"add-rcpt", func(_ context.Context, trx Trx) (Decision, error) {
			trx.AddRcptTo("someone@localhost", "")
			return Accept, nil
		}, []*milter.ModifyAction{{Type: milter.ActionAddRcpt, Rcpt: "someone@localhost"}}, false},
		{"add-rcpt-par", func(_ context.Context, trx Trx) (Decision, error) {
			trx.AddRcptTo("someone@localhost", "A=B")
			return Accept, nil
		}, []*milter.ModifyAction{{Type: milter.ActionAddRcpt, Rcpt: "someone@localhost", RcptArgs: "A=B"}}, false},
		{"add-rcpt-noop", func(_ context.Context, trx Trx) (Decision, error) {
			trx.AddRcptTo("root@localhost", "")
			return Accept, nil
		}, nil, false},
		{"add-rcpt-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			trx.AddRcptTo("someone@localhost", "")
			return Accept, nil
		}, nil, true},
		{"replace-rcpt", func(_ context.Context, trx Trx) (Decision, error) {
			trx.DelRcptTo("root@localhost")
			trx.AddRcptTo("someone@localhost", "")
			return Accept, nil
		}, []*milter.ModifyAction{
			{Type: milter.ActionDelRcpt, Rcpt: "root@localhost"},
			{Type: milter.ActionAddRcpt, Rcpt: "someone@localhost"},
		}, false},
		{"replace-body", func(_ context.Context, trx Trx) (Decision, error) {
			got, _ := io.ReadAll(trx.Body())
			if string(got) != "body" {
				t1.Fatalf("wrong body %q", got)
			}
			trx.ReplaceBody(io.NopCloser(strings.NewReader("test")))
			return Accept, nil
		}, []*milter.ModifyAction{
			{Type: milter.ActionReplaceBody, Body: []byte("test")},
		}, false},
		{"replace-body-buffered", func(_ context.Context, trx Trx) (Decision, error) {
			trx.ReplaceBody(strings.NewReader("test"))
			trx.Data()
			return Accept, nil
		}, []*milter.ModifyAction{
			{Type: milter.ActionReplaceBody, Body: []byte("test")},
		}, false},
		{"replace-body-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			trx.ReplaceBody(io.NopCloser(strings.NewReader("test")))
			return Accept, nil
		}, nil, true},
		{"replace-body-buffered-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			trx.ReplaceBody(io.NopCloser(strings.NewReader("test")))
			trx.Data()
			return Accept, nil
		}, nil, true},
		{"add-header", func(_ context.Context, trx Trx) (Decision, error) {
			trx.Headers().Add("X-Test", "1")
			return Accept, nil
		}, []*milter.ModifyAction{
			{Type: milter.ActionInsertHeader, HeaderIndex: 104, HeaderName: "X-Test", HeaderValue: " 1"},
		}, false},
		{"prepend-header", func(_ context.Context, trx Trx) (Decision, error) {
			f := trx.Headers().Fields()
			f.Next()
			f.InsertBefore("X-Test", "1")
			return Accept, nil
		}, []*milter.ModifyAction{
			{Type: milter.ActionInsertHeader, HeaderIndex: 1, HeaderName: "X-Test", HeaderValue: " 1"},
		}, false},
		{"prepend-header-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			f := trx.Headers().Fields()
			f.Next()
			f.InsertBefore("X-Test", "1")
			return Accept, nil
		}, nil, true},
		{"change-header", func(_ context.Context, trx Trx) (Decision, error) {
			f := trx.Headers().Fields()
			f.Next()
			f.SetAddressList([]*mail.Address{{Address: "root@localhost", Name: "root"}})
			return Accept, nil
		}, []*milter.ModifyAction{
			{Type: milter.ActionChangeHeader, HeaderIndex: 1, HeaderName: "From", HeaderValue: " \"root\" <root@localhost>"},
		}, false},
		{"change-header-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			f := trx.Headers().Fields()
			f.Next()
			f.SetAddressList([]*mail.Address{{Address: "root@localhost", Name: "root"}})
			return Accept, nil
		}, nil, true},
		{"quarantine", func(ctx context.Context, trx Trx) (Decision, error) {
			return QuarantineResponse("test"), nil
		}, []*milter.ModifyAction{
			{Type: milter.ActionQuarantine, Reason: "test"},
		}, false},
		{"quarantine-err", func(ctx context.Context, trx Trx) (Decision, error) {
			ctx.Value("s").(*mockSession).actErr = expectErr
			return QuarantineResponse("test"), nil
		}, nil, true},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			b, s := newMockBackend()
			t1.Cleanup(b.transaction.cleanup)
			_, _ = b.MailFrom("", "", s.mod)
			_, _ = b.RcptTo("root@localhost", "", s.mod)
			_, _ = b.Header("From", " <>", s.mod)
			_, _ = b.Header("To", " <root@localhost>", s.mod)
			_, _ = b.Header("Subject", " test", s.mod)
			_, _ = b.BodyChunk([]byte("body"), s.mod)
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
			if err := b.transaction.sendModifications(s.mod); (err != nil) != tt.wantErr {
				t1.Errorf("sendModifications() error = %v, wantErr %v", err, tt.wantErr)
			}
			got := s.modifications
			if !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("sendModifications() sent\n%swant\n%s", outputMessages(got), outputMessages(tt.want))
			}
		})
	}
}

func TestMTA_IsSendmail(t *testing.T) {
	type fields struct {
		Version string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{"vanilla", fields{"8.15.2"}, true},
		{"suffixed", fields{"8.15.2-ubuntu12"}, true},
		{"Postfix default", fields{"Postfix 8.15.2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MTA{
				Version: tt.fields.Version,
			}
			if got := m.IsSendmail(); got != tt.want {
				t.Errorf("IsSendmail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_transaction_HeadersEnforceOrder(t1 *testing.T) {
	type fields struct {
		mta MTA
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{"Postfix", fields{MTA{Version: "Postfix 3.1"}}, false},
		{"Future Postfix", fields{MTA{Version: "Postfix 8.4.4"}}, false},
		{"Sendmail", fields{MTA{Version: "8.4.4"}}, true},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &transaction{
				mta: tt.fields.mta,
			}
			t.HeadersEnforceOrder()
			if got := t.enforceHeaderOrder; got != tt.want {
				t1.Errorf("enforceHeaderOrder = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_transaction_Data(t1 *testing.T) {
	hdrs, _ := header.New([]byte("From: root@localhost\r\nTo: root@localhost\r\n\r\n"))
	bdy := func(data []byte) *body.Body {
		b := body.New(len(data), int64(len(data)))
		_, _ = b.Write(data)
		return b
	}
	type fields struct {
		origHeaders         *header.Header
		headers             *header.Header
		body                *body.Body
		bodyReplacement     io.Reader
		bufferedReplacement *body.Body
	}
	tests := []struct {
		name    string
		fields  fields
		want    []byte
		wantErr bool
	}{
		{"not-replaced", fields{hdrs.Copy(), hdrs.Copy(), bdy([]byte("test")), nil, nil}, []byte("From: root@localhost\r\nTo: root@localhost\r\n\r\ntest"), false},
		{"replaced", fields{hdrs.Copy(), hdrs.Copy(), bdy([]byte("test")), bytes.NewReader([]byte("test1")), nil}, []byte("From: root@localhost\r\nTo: root@localhost\r\n\r\ntest1"), false},
		{"replaced-buffered", fields{hdrs.Copy(), hdrs.Copy(), bdy([]byte("test")), bytes.NewReader([]byte("test1")), bdy([]byte("test2"))}, []byte("From: root@localhost\r\nTo: root@localhost\r\n\r\ntest2"), false},
		{"replaced-err", fields{hdrs.Copy(), hdrs.Copy(), bdy([]byte("test")), body.ErrReader{Err: io.ErrUnexpectedEOF}, nil}, []byte("From: root@localhost\r\nTo: root@localhost\r\n\r\n"), true},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &transaction{
				headers:             tt.fields.headers,
				origHeaders:         tt.fields.origHeaders,
				body:                tt.fields.body,
				bodyReplacement:     tt.fields.bodyReplacement,
				bufferedReplacement: tt.fields.bufferedReplacement,
			}
			r := t.Data()
			got, err := io.ReadAll(r)
			if (err != nil) != tt.wantErr {
				t1.Errorf("Data() error = %v, wantErr %v", err, tt.wantErr)
			}
			if bytes.Compare(got, tt.want) != 0 {
				t1.Errorf("Data() = %q, want %q", got, tt.want)
			}
		})
	}
}
