package mailfilter

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/internal/header"
	"github.com/d--j/go-milter/internal/wire"
	"github.com/d--j/go-milter/mailfilter/addr"
)

type mockSession struct {
	modifications              []*wire.Message
	progressCalled             int
	macros                     *milter.MacroBag
	progressErr                error
	WritePacket, WriteProgress func(msg *wire.Message) error
}

func (s *mockSession) writePacket(msg *wire.Message) error {
	s.modifications = append(s.modifications, msg)
	return nil
}

func (s *mockSession) writeProgress(_ *wire.Message) error {
	s.progressCalled++
	return s.progressErr
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
	body := bodyOption{
		MaxMem:    200 * 1024,
		MaxSize:   100 * 1024 * 1024,
		MaxAction: TruncateWhenTooBig,
	}
	return &backend{
		opts: options{
			decisionAt:    DecisionAtEndOfMessage,
			errorHandling: Error,
			body:          &body,
			header: &headerOption{
				Max:       512,
				MaxAction: TruncateWhenTooBig,
			},
		},
		leadingSpace: false,
		decision:     nil,
		transaction:  &transaction{bodyOpt: body},
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
	trx := transaction{connect: Connect{Host: "host"}, helo: Helo{Name: "name"}}
	b.transaction = &trx
	if err := b.Abort(s.newModifier()); err != nil {
		t.Errorf("expected nil, got %s", err)
	}
	if b.transaction == &trx {
		t.Errorf("expected new transaction")
	}
	if b.transaction.Connect().Host != "host" || b.transaction.Helo().Name != "name" {
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
	data, _ := io.ReadAll(b.transaction.body)
	b.transaction.cleanup()
	if string(data) != "testtest" {
		t.Fatalf("got %q, expected %q", data, "testtest")
	}
}

func Test_backend_BodyChunkMaxReject(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.body.MaxSize = 6
	b.opts.body.MaxAction = RejectMessageWhenTooBig
	b.transaction.bodyOpt = *b.opts.body
	resp, err := b.BodyChunk([]byte("test"), s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.BodyChunk([]byte("test"), s.newModifier())
	if err != nil {
		t.Fatalf("got err %s", err)
	}
	if resp == nil {
		t.Fatalf("got nil resp")
	}
	want := "response=reply_code action=reject code=552 reason=\"552 5.3.4 Maximum allowed body size of 6 bytes exceeded.\""
	if resp.String() != want {
		t.Fatalf("got resp %s, expected %s", resp.String(), want)
	}
}

func Test_backend_BodyChunkMaxTruncate(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.body.MaxSize = 6
	b.opts.body.MaxAction = TruncateWhenTooBig
	b.transaction.bodyOpt = *b.opts.body
	resp, err := b.BodyChunk([]byte("test"), s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.BodyChunk([]byte("test"), s.newModifier())
	assertContinue(t, resp, err)
	if b.transaction.body == nil {
		t.Fatal("body file is nil")
	}
	_, _ = b.transaction.body.Seek(0, io.SeekStart)
	data, _ := io.ReadAll(b.transaction.body)
	b.transaction.cleanup()
	if string(data) != "testte" {
		t.Fatalf("got %q, expected %q", data, "testte")
	}
}

func Test_backend_BodyChunkMaxClear(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.body.MaxSize = 6
	b.opts.body.MaxAction = ClearWhenTooBig
	b.transaction.bodyOpt = *b.opts.body
	resp, err := b.BodyChunk([]byte("test"), s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.BodyChunk([]byte("test"), s.newModifier())
	assertContinue(t, resp, err)
	if b.transaction.body == nil {
		t.Fatal("body file is nil")
	}
	_, _ = b.transaction.body.Seek(0, io.SeekStart)
	data, _ := io.ReadAll(b.transaction.body)
	b.transaction.cleanup()
	if string(data) != "" {
		t.Fatalf("got %q, expected %q", data, "")
	}
}

func Test_backend_Cleanup(t *testing.T) {
	t.Parallel()
	b, _ := newMockBackend()
	trx := transaction{}
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
	expect := &Connect{
		Host:   "host",
		Family: "family",
		Port:   123,
		Addr:   "127.0.0.2",
		IfName: "ifname",
		IfAddr: "127.0.0.3",
	}
	got := b.transaction.Connect()
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
	got := b.transaction.QueueId()
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("Data() = %q, expected %q", got, expect)
	}
}

func Test_backend_EndOfMessage(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	expectedErr := errors.New("error")
	b.decision = func(_ context.Context, trx Trx) (Decision, error) {
		if trx.QueueId() != "Q123" {
			t.Fatalf("queueId = %q, expected %q", trx.QueueId(), "Q123")
		}
		return nil, expectedErr
	}
	resp, err := b.EndOfMessage(s.newModifier())
	if resp != nil || err != expectedErr {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.transaction.addHeader("subject", []byte("subject: test"))
	b.decision = func(_ context.Context, trx Trx) (Decision, error) {
		if subj := trx.Headers().Value("Subject"); subj != " test" {
			t.Fatalf("Subject = %q, expected %q", subj, " test")
		}
		return nil, expectedErr
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != nil || err != expectedErr {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
		return TempFail, nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespTempFail || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
		return Reject, nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespReject || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
		return Discard, nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespDiscard || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
		return CustomErrorResponse(400, "not right now"), nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp == nil || resp.Response().Code != wire.Code(wire.ActReplyCode) || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
	b.Cleanup()
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
		return CustomErrorResponse(200, "not right now"), nil
	}
	resp, err = b.EndOfMessage(s.newModifier())
	if resp != milter.RespTempFail || err != nil {
		t.Fatalf("wrong return %v, %v", resp, err)
	}
}

func outputFields(hdr *header.Header) string {
	bytes, _ := io.ReadAll(hdr.Reader())
	return string(bytes)
}

func Test_backend_Header(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.Header("from", "root", s.newModifier())
	assertContinue(t, resp, err)
	b.leadingSpace = true
	resp, err = b.Header("To", " root, 1", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 2", s.newModifier())
	assertContinue(t, resp, err)
	b.leadingSpace = false
	resp, err = b.Header("To", "\troot, 3", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 4", s.newModifier())
	assertContinue(t, resp, err)
	expect, err := header.New([]byte("from: root\r\nTo: root, 1\r\nTo:root, 2\r\nTo:\troot, 3\r\nTo: root, 4\n\r\n"))
	if err != nil {
		panic(err)
	}
	got := b.transaction.origHeaders
	if outputFields(got) != outputFields(expect) {
		t.Fatalf("Header() = %q, expected %q", outputFields(got), outputFields(expect))
	}
}

func Test_backend_HeaderMaxReject(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.header.Max = 2
	b.opts.header.MaxAction = RejectMessageWhenTooBig
	resp, err := b.Header("from", "root", s.newModifier())
	assertContinue(t, resp, err)
	b.leadingSpace = true
	resp, err = b.Header("To", " root, 1", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 2", s.newModifier())
	if err != nil {
		t.Fatalf("got err %s", err)
	}
	if resp == nil {
		t.Fatalf("got nil resp")
	}
	want := "response=reply_code action=reject code=552 reason=\"552 5.3.4 Maximum allowed header lines (2) exceeded.\""
	if resp.String() != want {
		t.Fatalf("got resp %s, expected %s", resp.String(), want)
	}
}

func Test_backend_HeaderMaxTruncate(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.header.Max = 2
	b.opts.header.MaxAction = TruncateWhenTooBig
	resp, err := b.Header("from", "root", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 1", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 2", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 3", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 4", s.newModifier())
	assertContinue(t, resp, err)
	expect, err := header.New([]byte("from: root\r\nTo: root, 1\r\n\r\n"))
	if err != nil {
		panic(err)
	}
	got := b.transaction.origHeaders
	if outputFields(got) != outputFields(expect) {
		t.Fatalf("Header() = %q, expected %q", outputFields(got), outputFields(expect))
	}
}

func Test_backend_HeaderMaxClear(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.header.Max = 2
	b.opts.header.MaxAction = ClearWhenTooBig
	resp, err := b.Header("from", "root", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 1", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 2", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 3", s.newModifier())
	assertContinue(t, resp, err)
	resp, err = b.Header("To", "root, 4", s.newModifier())
	assertContinue(t, resp, err)
	expect, err := header.New([]byte("\r\n"))
	if err != nil {
		panic(err)
	}
	got := b.transaction.origHeaders
	if outputFields(got) != outputFields(expect) {
		t.Fatalf("Header() = %q, expected %q", outputFields(got), outputFields(expect))
	}
}
func Test_backend_Headers(t *testing.T) {
}

func Test_backend_Helo(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.Helo("helohost", s.newModifier())
	assertContinue(t, resp, err)
	expect := &Helo{
		Name:        "helohost",
		TlsVersion:  "tls-version",
		Cipher:      "cipher",
		CipherBits:  "cipher-bits",
		CertSubject: "cert-subject",
		CertIssuer:  "cert-issuer",
	}
	got := b.transaction.Helo()
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("Helo() = %v, expected %v", got, expect)
	}
}

func Test_backend_MailFrom(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.MailFrom("root@localhost", "A=B", s.newModifier())
	assertContinue(t, resp, err)
	expect := addr.NewMailFrom("root@localhost", "A=B", "mail-mailer", "auth-authen", "auth-type")
	got := b.transaction.origMailFrom
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
	expect := []*addr.RcptTo{
		addr.NewRcptTo("root@localhost", "A=B", "rcpt-mailer"),
		addr.NewRcptTo("nobody@localhost", "", "2"),
	}
	got := b.transaction.origRcptTos
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("RcptTo() = %v, expected %v", got, expect)
	}
}

func Test_backend_RcptToReject(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.rcptToValidator = func(_ context.Context, in *RcptToValidationInput) (Decision, error) {
		in.MTA.Daemon = "change in copy"
		if in.RcptTo.Addr == "root@localhost" {
			return Accept, nil
		}
		time.Sleep(time.Second * 2)
		return Reject, nil
	}
	resp, err := b.RcptTo("root@localhost", "A=B", s.newModifier())
	assertContinue(t, resp, err)
	s.macros.Set(milter.MacroRcptMailer, "2")
	resp, err = b.RcptTo("nobody@localhost", "", s.newModifier())
	if err != nil {
		t.Fatalf("got err %s", err)
	}
	if resp != milter.RespReject {
		t.Fatalf("got resp %v expected reject", resp)
	}
	expect := []*addr.RcptTo{
		addr.NewRcptTo("root@localhost", "A=B", "rcpt-mailer"),
	}
	got := b.transaction.origRcptTos
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("RcptTo() = %v, expected %v", got, expect)
	}
	if s.progressCalled < 1 {
		t.Fatalf("progress not called")
	}
	if b.transaction.MTA().Daemon == "change in copy" {
		t.Fatalf("MTA was not copied to rcptToValidator")
	}
}

func Test_backend_RcptToDiscard(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.rcptToValidator = func(_ context.Context, in *RcptToValidationInput) (Decision, error) {
		if in.RcptTo.Addr == "root@localhost" {
			return Accept, nil
		}
		time.Sleep(time.Second * 2)
		return Discard, nil
	}
	resp, err := b.RcptTo("root@localhost", "A=B", s.newModifier())
	assertContinue(t, resp, err)
	s.macros.Set(milter.MacroRcptMailer, "2")
	resp, err = b.RcptTo("nobody@localhost", "", s.newModifier())
	if err != nil {
		t.Fatalf("got err %s", err)
	}
	if resp != milter.RespDiscard {
		t.Fatalf("got resp %v expected discard", resp)
	}
	expect := []*addr.RcptTo{
		addr.NewRcptTo("root@localhost", "A=B", "rcpt-mailer"),
	}
	got := b.transaction.origRcptTos
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("RcptTo() = %v, expected %v", got, expect)
	}
	if s.progressCalled < 1 {
		t.Fatalf("progress not called")
	}
}

func Test_backend_RcptToValidationError(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.rcptToValidator = func(ctx context.Context, in *RcptToValidationInput) (Decision, error) {
		if in.RcptTo.Addr == "root@localhost" {
			return Accept, nil
		}
		return nil, errors.New("error")
	}
	resp, err := b.RcptTo("root@localhost", "A=B", s.newModifier())
	assertContinue(t, resp, err)
	s.macros.Set(milter.MacroRcptMailer, "2")
	resp, err = b.RcptTo("nobody@localhost", "", s.newModifier())
	if err == nil {
		t.Fatalf("got err nil, expected error")
	}
}

func Test_backend_RcptToProgressError(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	b.opts.rcptToValidator = func(ctx context.Context, in *RcptToValidationInput) (Decision, error) {
		if in.RcptTo.Addr == "root@localhost" {
			return Accept, nil
		}
		select {
		case <-time.After(time.Second * 2):
			return Discard, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.progressErr = errors.New("error")
	resp, err := b.RcptTo("root@localhost", "A=B", s.newModifier())
	assertContinue(t, resp, err)
	s.macros.Set(milter.MacroRcptMailer, "2")
	resp, err = b.RcptTo("nobody@localhost", "", s.newModifier())
	if err == nil {
		t.Fatalf("got err nil, expected context done")
	}
	if s.progressCalled < 1 {
		t.Fatalf("progress not called")
	}
}

func Test_backend_decideOrContinue(t *testing.T) {
	t.Parallel()
	b, s := newMockBackend()
	resp, err := b.decideOrContinue(DecisionAtHelo, s.newModifier())
	assertContinue(t, resp, err)
	b.opts.decisionAt = DecisionAtHelo
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
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
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
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
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
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
	b.decision = func(_ context.Context, _ Trx) (Decision, error) {
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
