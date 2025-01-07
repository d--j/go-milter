package testtrx

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/d--j/go-milter/internal/header"
	"github.com/d--j/go-milter/mailfilter"
	"github.com/d--j/go-milter/mailfilter/addr"
)

func TestTestTrx(t *testing.T) {
	t.Parallel()
	hdr, err := header.New([]byte("Subject: test\nX-H: 1\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	trx := (&Trx{}).SetMTA(mailfilter.MTA{
		Version: "Postfix 2.0.0",
		FQDN:    "mx.example.net",
		Daemon:  "smtpd",
	}).SetConnect(mailfilter.Connect{
		Host:   "localhost",
		Family: "tcp",
		Port:   25,
		Addr:   "127.0.0.1",
		IfName: "lo",
		IfAddr: "127.0.0.1",
	}).SetHelo(mailfilter.Helo{
		Name:        "localhost",
		TlsVersion:  "",
		Cipher:      "",
		CipherBits:  "",
		CertSubject: "",
		CertIssuer:  "",
	}).
		SetQueueId("ABCD").
		SetMailFrom(addr.NewMailFrom("root@localhost", "", "local", "", "")).
		SetRcptTosList("root@localhost", "postmaster@example.com").
		SetHeaders(hdr).
		SetBodyBytes([]byte("test body"))

	if trx.MailFrom() == nil || trx.MailFrom().Addr != "root@localhost" {
		t.Errorf("MailFrom expected to be root@localhost, got %v", trx.MailFrom())
	}
	trx.HeadersEnforceOrder()
	if trx.enforceHeaderOrder == true {
		t.Errorf("HeadersEnforceOrder expected do nothing")
	}
	if trx.HasRcptTo("postmaster@example.net") {
		t.Errorf("HasRcptTo postmaster@example.net expected false")
	}
	if len(trx.RcptTos()) != 2 {
		t.Errorf("expected 2 RcptTos, got %v", len(trx.RcptTos()))
	}
	if trx.QueueId() != "ABCD" {
		t.Errorf("QueueId expected ABCD, got %v", trx.QueueId())
	}

	m := trx.Modifications()
	if m != nil || len(m) != 0 {
		t.Fatalf("trx.Modification() got %v, want <nil>", m)
	}

	trx.ChangeMailFrom("", "A=B")
	trx.DelRcptTo("root@localhost")
	trx.AddRcptTo("postmaster@example.com", "A=B")
	trx.AddRcptTo("postmaster@example.net", "")
	trx.AddRcptTo("", "")
	trx.Headers().Add("X-Add", "1")
	trx.Headers().SetSubject("")
	trx.ReplaceBody(bytes.NewReader([]byte("new body")))

	m = trx.Modifications()
	expected := []Modification{
		{Kind: ChangeFrom, Addr: "", Args: "A=B"},
		{Kind: DelRcptTo, Addr: "postmaster@example.com"},
		{Kind: DelRcptTo, Addr: "root@localhost"},
		{Kind: AddRcptTo, Addr: "postmaster@example.com", Args: "A=B"},
		{Kind: AddRcptTo, Addr: "postmaster@example.net", Args: ""},
		{Kind: AddRcptTo, Addr: "", Args: ""},
		{Kind: ChangeHeader, Index: 1, Name: "Subject", Value: ""},
		{Kind: InsertHeader, Index: 104, Name: "X-Add", Value: " 1"},
		{Kind: ReplaceBody, Body: []byte("new body")},
	}
	if !reflect.DeepEqual(m, expected) {
		t.Fatalf("trx.Modifications() = %+v, want %+v", m, expected)
	}
}
