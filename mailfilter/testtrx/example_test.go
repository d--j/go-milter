package testtrx_test

import (
	"context"
	"fmt"

	"github.com/d--j/go-milter/mailfilter"
	"github.com/d--j/go-milter/mailfilter/addr"
	"github.com/d--j/go-milter/mailfilter/testtrx"
)

func Filter(_ context.Context, trx mailfilter.Trx) (mailfilter.Decision, error) {
	trx.ChangeMailFrom("", "A=B")
	trx.Log("test")
	return mailfilter.Accept, nil
}

func ExampleTrx() {
	trx := (&testtrx.Trx{}).SetMTA(mailfilter.MTA{
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
		SetHeadersRaw([]byte("Subject: test\n\n")).
		SetBodyBytes([]byte("test body"))

	decision, _ := Filter(context.Background(), trx)
	if decision != mailfilter.Accept {
		fmt.Println("decision wrong")
	}
	for _, m := range trx.Modifications() {
		fmt.Println(m)
	}
	fmt.Println(trx.Logs())

	// Output: {0  A=B 0   []}
	// [test]
}
