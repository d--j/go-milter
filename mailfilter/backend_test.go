package mailfilter

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/internal/wire"
)

type mockSession struct {
	modifications              []*wire.Message
	progressCalled             int
	macros                     *milter.MacroBag
	WritePacket, WriteProgress func(msg *wire.Message) error
}

func (s *mockSession) writePacket(msg *wire.Message) error {
	s.modifications = append(s.modifications, msg)
	return nil
}

func (s *mockSession) writeProgress(_ *wire.Message) error {
	s.progressCalled++
	return nil
}

func (s *mockSession) newModifier() *milter.Modifier {
	if s.macros == nil {
		m := milter.NewMacroBag()
		m.Set(milter.MacroIfName, "ifname")
		m.Set(milter.MacroIfAddr, "127.0.0.3")
		m.Set(milter.MacroTlsVersion, "tls-version")
		m.Set(milter.MacroCipher, "cipher")
		m.Set(milter.MacroCipherBits, "cipher-bits")
		m.Set(milter.MacroCertSubject, "cert-subject")
		m.Set(milter.MacroCertIssuer, "cert-issuer")
		m.Set(milter.MacroMailMailer, "mail-mailer")
		m.Set(milter.MacroAuthAuthen, "auth-authen")
		m.Set(milter.MacroAuthType, "auth-type")
		m.Set(milter.MacroRcptMailer, "rcpt-mailer")
		m.Set(milter.MacroQueueId, "Q123")
		s.macros = m
	}
	if s.WritePacket == nil {
		s.WritePacket = s.writePacket
	}
	if s.WriteProgress == nil {
		s.WriteProgress = s.writeProgress
	}
	return milter.NewTestModifier(s.macros, s.WritePacket, s.WriteProgress, milter.AllClientSupportedActionMasks, milter.DataSize64K)
}

func newMockBackend() (*backend, *mockSession) {
	return &backend{
		opts: options{
			decisionAt:    DecisionAtEndOfMessage,
			errorHandling: Error,
		},
		leadingSpace: false,
		decision:     nil,
		transaction:  &Transaction{},
	}, &mockSession{}
}

func assertContinue(t *testing.T, resp *milter.Response, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("got err %s", err)
	}
	if resp != milter.RespContinue {
		t.Fatalf("got resp %v expected continue", resp)
	}
}

func Test_backend_Abort(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	trx := Transaction{Connect: Connect{Host: "host"}, Helo: Helo{Name: "name"}}
	b.transaction = &trx
	if err := b.Abort(s.newModifier()); err != nil {
		t.Errorf("expected nil, got %s", err)
	}
	if b.transaction == &trx {
		t.Errorf("expected new transaction")
	}
	if b.transaction.Connect.Host != "host" || b.transaction.Helo.Name != "name" {
		t.Errorf("expected Connect and Helo to persist")
	}
	b.transaction = nil
	if err := b.Abort(s.newModifier()); err != nil {
		t.Errorf("expected nil, got %s", err)
	}
	if b.transaction == nil {
		t.Errorf("expected new transaction")
	}
}

func Test_backend_BodyChunk(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.BodyChunk([]byte("test"), s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.BodyChunk([]byte("test"), s.newModifier())
	assertContinue(t, resp, err)
	if b.transaction.body == nil {
		t.Fatal("body file is nil")
	}
	_, _ = b.transaction.body.Seek(0, io.SeekStart)
	data, err := io.ReadAll(b.transaction.body)
	b.transaction.cleanup()
	if string(data) != "testtest" {
		t.Fatalf("got %q, expected %q", data, "testtest")
	}
}

func Test_backend_Cleanup(t *testing.T) {
	t.Parallel()
	b, _ := newMockBackend()
	trx := Transaction{}
	b.transaction = &trx
	b.Cleanup()
	if b.transaction == &trx {
		t.Errorf("expected new transaction")
	}
}

func Test_backend_Connect(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.Connect("host", "family", 123, "127.0.0.2", s.newModifier())
	assertContinue(t, resp, err)
	expect := Connect{
		Host:   "host",
		Family: "family",
		Port:   123,
		Addr:   "127.0.0.2",
		IfName: "ifname",
		IfAddr: "127.0.0.3",
	}
	got := b.transaction.Connect
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("Connect() = %v, expected %v", got, expect)
	}
}

func Test_backend_Data(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.Data(s.newModifier())
	assertContinue(t, resp, err)
	expect := "Q123"
	got := b.transaction.QueueId
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("Data() = %q, expected %q", got, expect)
	}
}

func Test_backend_EndOfMessage(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	expectedErr := errors.New("error")
	b.decision = func(_ context.Context, trx *Transaction) (Decision, error) {
		if trx.QueueId != "Q123" {
			t.Fatalf("QueueId = %q, expected %q", trx.QueueId, "Q123")
		}
		return nil, expectedErr
	}
	resp, err := b.EndOfMessage(s.newModifier())
	if resp != nil || err != expectedErr {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.transaction.addHeader("subject", "subject: test")
	b.decision = func(_ context.Context, trx *Transaction) (Decision, error) {
		if subj := trx.Headers.Get("Subject"); subj != " test" {
			t.Fatalf("Subject = %q, expected %q", subj, " test")
		}
		return nil, expectedErr
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != nil || err != expectedErr {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, trx *Transaction) (Decision, error) {
		return TempFail, nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespTempFail || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, trx *Transaction) (Decision, error) {
		return Reject, nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespReject || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, trx *Transaction) (Decision, error) {
		return Discard, nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespDiscard || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, trx *Transaction) (Decision, error) {
		return CustomErrorResponse(400, "not right now"), nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp == nil || resp.Response().Code != wire.Code(wire.ActReplyCode) || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, trx *Transaction) (Decision, error) {
		return CustomErrorResponse(200, "not right now"), nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespTempFail || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
}

func Test_backend_Header(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.Header("from", "root", s.newModifier())
	assertContinue(t, resp, err)
	b.leadingSpace = true
	resp, err = b.Header("To", " root, nobody", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, nobody", s.newModifier())
	assertContinue(t, resp, err)
	b.leadingSpace = false
	resp, err = b.Header("To", "\troot, nobody", s.newModifier())
	assertContinue(t, resp, err)
	expect := []*headerField{
		{0, "From", "from: root"},
		{1, "To", "To: root, nobody"},
		{2, "To", "To: root, nobody"},
		{3, "To", "To:\troot, nobody"},
	}
	got := b.transaction.headers.fields
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("Header() = %s, expected %s", outputFields(got), outputFields(expect))
	}
}

func Test_backend_Headers(t *testing.T) {
}

func Test_backend_Helo(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.Helo("helohost", s.newModifier())
	assertContinue(t, resp, err)
	expect := Helo{
		Name:        "helohost",
		TlsVersion:  "tls-version",
		Cipher:      "cipher",
		CipherBits:  "cipher-bits",
		CertSubject: "cert-subject",
		CertIssuer:  "cert-issuer",
	}
	got := b.transaction.Helo
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("Helo() = %v, expected %v", got, expect)
	}
}

func Test_backend_MailFrom(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.MailFrom("root@localhost", "A=B", s.newModifier())
	assertContinue(t, resp, err)
	expect := MailFrom{
		addr:                 addr{Addr: "root@localhost", Args: "A=B"},
		transport:            "mail-mailer",
		authenticatedUser:    "auth-authen",
		authenticationMethod: "auth-type",
	}
	got := b.transaction.mailFrom
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("MailFrom() = %v, expected %v", got, expect)
	}
}

func Test_backend_RcptTo(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.RcptTo("root@localhost", "A=B", s.newModifier())
	assertContinue(t, resp, err)
	s.macros.Set(milter.MacroRcptMailer, "2")
	resp, err = b.RcptTo("nobody@localhost", "", s.newModifier())
	assertContinue(t, resp, err)
	expect := []RcptTo{{
		addr:      addr{Addr: "root@localhost", Args: "A=B"},
		transport: "rcpt-mailer",
	}, {
		addr:      addr{Addr: "nobody@localhost", Args: ""},
		transport: "2",
	}}
	got := b.transaction.rcptTos
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("RcptTo() = %v, expected %v", got, expect)
	}
}

func Test_backend_decideOrContinue(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.decideOrContinue(DecisionAtHelo, s.newModifier())
	assertContinue(t, resp, err)
	b.opts.decisionAt = DecisionAtHelo
	b.decision = func(ctx context.Context, trx *Transaction) (Decision, error) {
		return Accept, nil
	}
	resp, err = b.decideOrContinue(DecisionAtHelo, s.newModifier())
	if err != nil {
		t.Fatalf("got err %s", err)
	}
	if resp != milter.RespAccept {
		t.Fatalf("got resp %v expected accept", resp)
	}
	b.Cleanup()
	b.decision = func(ctx context.Context, trx *Transaction) (Decision, error) {
		return nil, io.EOF
	}
	_, err = b.decideOrContinue(DecisionAtHelo, s.newModifier())
	if err != io.EOF {
		t.Fatalf("got err %v, want io.EOF", err)
	}
}

func Test_backend_error(t *testing.T) {
	savedWarning := milter.LogWarning
	defer func() {
		milter.LogWarning = savedWarning
	}()
	warningCalled := 0
	milter.LogWarning = func(_ string, _ ...interface{}) {
		warningCalled++
	}
	expected := errors.New("error")
	b, _ := newMockBackend()
	resp, err := b.error(expected)
	if err != expected || resp != nil {
		t.Fatalf("error() wrong return values %v, %v", resp, err)
	}
	if warningCalled != 0 {
		t.Fatalf("wrong warningCalled value %d", warningCalled)
	}
	b.opts.errorHandling = AcceptWhenError
	resp, err = b.error(expected)
	if err != expected || resp != milter.RespAccept {
		t.Fatalf("error() wrong return values %v, %v", resp, err)
	}
	if warningCalled != 1 {
		t.Fatalf("wrong warningCalled value %d", warningCalled)
	}
	b.opts.errorHandling = TempFailWhenError
	resp, err = b.error(expected)
	if err != expected || resp != milter.RespTempFail {
		t.Fatalf("error() wrong return values %v, %v", resp, err)
	}
	if warningCalled != 2 {
		t.Fatalf("wrong warningCalled value %d", warningCalled)
	}
	b.opts.errorHandling = RejectWhenError
	resp, err = b.error(expected)
	if err != expected || resp != milter.RespReject {
		t.Fatalf("error() wrong return values %v, %v", resp, err)
	}
	if warningCalled != 3 {
		t.Fatalf("wrong warningCalled value %d", warningCalled)
	}

	defer func() { _ = recover() }()
	b.opts.errorHandling = 99
	_, _ = b.error(expected)
	t.Errorf("did not panic")
}

func Test_backend_makeDecision(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.decision = func(ctx context.Context, trx *Transaction) (Decision, error) {
		return Accept, nil
	}
	b.makeDecision(s.newModifier())
	if b.transaction.decision != Accept || b.transaction.decisionErr != nil {
		t.Fatal("values not set")
	}
	if s.progressCalled > 0 {
		t.Fatal("progress called")
	}
	b.Cleanup()
	b.decision = func(ctx context.Context, trx *Transaction) (Decision, error) {
		time.Sleep(time.Second + 30*time.Millisecond)
		return Accept, nil
	}
	b.makeDecision(s.newModifier())
	if b.transaction.decision != Accept || b.transaction.decisionErr != nil {
		t.Fatal("values not set")
	}
	if s.progressCalled != 1 {
		t.Fatal("progress not called")
	}
	b.Cleanup()
	expect := errors.New("error")
	s.WriteProgress = func(_ *wire.Message) error {
		return expect
	}
	b.makeDecision(s.newModifier())
	if b.transaction.decision != Accept || b.transaction.decisionErr != expect {
		t.Fatal("values not set")
	}
}
