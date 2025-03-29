package testtrx

import (
	"bytes"
	"github.com/d--j/go-milter/internal/body"
	"io"
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

	if trx.MTA() == nil || trx.MTA().FQDN != "mx.example.net" {
		t.Errorf("MTA.FQDN expected to be mx.example.net, got %v", trx.MTA().FQDN)
	}
	if trx.Connect() == nil || trx.Connect().Addr != "127.0.0.1" {
		t.Errorf("Connect.Addr expected to be 127.0.0.1, got %v", trx.Connect().Addr)
	}
	if trx.Helo() == nil || trx.Helo().Name != "localhost" {
		t.Errorf("Helo.Name expected to be localhost, got %v", trx.Helo().Name)
	}
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
	if diff := DiffModifications(expected, m); diff != "" {
		t.Fatalf("trxSendmail.Modifications() diff\n%s", diff)
	}

	trxSendmail := (&Trx{}).SetMTA(mailfilter.MTA{
		Version: "8.0.0",
		FQDN:    "mx.example.net",
		Daemon:  "smtpd",
	}).SetHeadersRaw([]byte("Subject: test\nX-H: 1\n\n"))
	trxSendmail.HeadersEnforceOrder()
	if trxSendmail.enforceHeaderOrder != true {
		t.Fatalf("HeadersEnforceOrder expected set the flag")
	}
	trxSendmail.Headers().Add("X-Add", "1")
	m = trxSendmail.Modifications()
	expected = []Modification{
		{Kind: ChangeHeader, Index: 1, Name: "X-H", Value: ""},
		{Kind: ChangeHeader, Index: 1, Name: "Subject", Value: ""},
		{Kind: InsertHeader, Index: 102, Name: "Subject", Value: " test"},
		{Kind: InsertHeader, Index: 103, Name: "X-H", Value: " 1"},
		{Kind: InsertHeader, Index: 104, Name: "X-Add", Value: " 1"},
	}
	if diff := DiffModifications(expected, m); diff != "" {
		t.Fatalf("trxSendmail.Modifications() diff\n%s", diff)
	}
}

func TestTrx_Data(t *testing.T) {
	const headers = "Subject: test\r\nX-H: 1\r\n\r\n"
	var two = 2
	type fields struct {
		changeHeader    bool
		body            []byte
		bodyReplacement []byte
		maxMem          *int
		callData        bool
	}
	tests := []struct {
		name   string
		fields fields
		want   []byte
	}{
		{"no-replace-no-body", fields{false, nil, nil, nil, true}, []byte{}},
		{"no-replace-body", fields{false, []byte("test"), nil, nil, true}, []byte("test")},
		{"replace-no-body", fields{false, nil, []byte("test"), nil, true}, []byte("test")},
		{"replace-and-body", fields{false, []byte("test"), []byte("test1"), nil, true}, []byte("test1")},
		{"replace-no-body-wo-data", fields{false, nil, []byte("test"), nil, false}, []byte("test")},
		{"replace-and-body-wo-data", fields{false, []byte("test"), []byte("test1"), nil, false}, []byte("test1")},
		{"change-header", fields{true, []byte("test"), []byte("test1"), nil, true}, []byte("test1")},
		{"use-temp-file", fields{true, []byte("test"), []byte("test1"), &two, true}, []byte("test1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr, _ := header.New([]byte(headers))
			trx := &Trx{
				origHeader: hdr,
				MaxMem:     tt.fields.maxMem,
			}
			trx.header = trx.origHeader.Copy()
			if tt.fields.changeHeader {
				trx.header.Set("X-Change", "1")
			}
			if tt.fields.body != nil {
				b := body.New(len(tt.fields.body), int64(len(tt.fields.body)))
				_, _ = b.Write(tt.fields.body)
				trx.body = b
			}
			if tt.fields.bodyReplacement != nil {
				trx.bodyReplacement = bytes.NewReader(tt.fields.bodyReplacement)
			}
			r := trx.Data()
			if r == nil {
				t.Fatal("Data() returned nil")
			}
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("Data() returned error: %v", err)
			}
			want, _ := io.ReadAll(trx.Headers().Reader())
			want = append(want, tt.want...)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Data() = %q, want %q", got, want)
			}
			if tt.fields.bodyReplacement != nil {
				m := trx.Modifications()
				// filter out all modifications except ReplaceBody since that's the only one we care about here
				filteredM := make([]Modification, 0, len(m))
				for _, mod := range m {
					if mod.Kind != ReplaceBody {
						continue
					}
					filteredM = append(filteredM, mod)
				}
				expected := []Modification{
					{Kind: ReplaceBody, Body: tt.fields.bodyReplacement},
				}
				if diff := DiffModifications(expected, filteredM); diff != "" {
					t.Fatalf("trx.Modifications() diff\n%s", diff)
				}
			}
		})
	}
}
