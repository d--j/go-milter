// Package testtrx can be used to test mailfilter based filter functions
package testtrx

import (
	"bytes"
	"fmt"
	"io"

	"github.com/d--j/go-milter/internal/header"
	"github.com/d--j/go-milter/internal/rcptto"
	"github.com/d--j/go-milter/mailfilter"
	"github.com/d--j/go-milter/mailfilter/addr"
	header2 "github.com/d--j/go-milter/mailfilter/header"
	"github.com/d--j/go-milter/milterutil"
	"golang.org/x/text/transform"
)

type ModificationKind int

const (
	ChangeFrom   ModificationKind = iota // a change mail from modification would have been sent to the MTA
	AddRcptTo                            // an add-recipient modification would have been sent to the MTA
	DelRcptTo                            // a delete-recipient modification would have been sent to the MTA
	InsertHeader                         // an insert-header modification would have been sent to the MTA
	ChangeHeader                         // a change-header modification would have been sent to the MTA
	ReplaceBody                          // a replace-body modification would have been sent to the MTA
)

// Modification is a modification that a [mailfilter.DecisionModificationFunc] made to the Trx.
type Modification struct {
	Kind  ModificationKind
	Addr  string
	Args  string
	Index int
	Name  string
	Value string
	Body  []byte
}

// Trx implements [mailfilter.Trx] for unit tests.
// Use this struct when you want to test your decision functions.
// You can use the fluent Set* methods of this struct to build up the transaction you want to test.
// After you passed the Trx to your decision function, you can call [Trx.Modifications] and [Trx.Log] to
// check that your decision function did what was expected of it.
type Trx struct {
	mta             mailfilter.MTA
	connect         mailfilter.Connect
	helo            mailfilter.Helo
	mailFrom        addr.MailFrom
	origMailFrom    addr.MailFrom
	rcptTos         []*addr.RcptTo
	origRcptTos     []*addr.RcptTo
	queueId         string
	logs            []string
	header          *header.Header
	origHeader      *header.Header
	body            io.ReadSeeker
	bodyReplacement io.Reader
}

func (t *Trx) MTA() *mailfilter.MTA {
	return &t.mta
}

func (t *Trx) SetMTA(mta mailfilter.MTA) *Trx {
	t.mta = mta
	return t
}

func (t *Trx) Connect() *mailfilter.Connect {
	return &t.connect
}

func (t *Trx) SetConnect(connect mailfilter.Connect) *Trx {
	t.connect = connect
	return t
}

func (t *Trx) Helo() *mailfilter.Helo {
	return &t.helo
}

func (t *Trx) SetHelo(helo mailfilter.Helo) *Trx {
	t.helo = helo
	return t
}

func (t *Trx) MailFrom() *addr.MailFrom {
	return &t.mailFrom
}

func (t *Trx) SetMailFrom(mailFrom addr.MailFrom) *Trx {
	t.mailFrom = mailFrom
	t.origMailFrom = mailFrom
	return t
}

func (t *Trx) ChangeMailFrom(from string, esmtpArgs string) {
	t.mailFrom.Addr = from
	t.mailFrom.Args = esmtpArgs
}

func (t *Trx) RcptTos() []*addr.RcptTo {
	return t.rcptTos
}

func (t *Trx) SetRcptTos(rcptTos []*addr.RcptTo) *Trx {
	t.rcptTos = rcptto.Copy(rcptTos)
	t.origRcptTos = rcptto.Copy(rcptTos)
	return t
}

func (t *Trx) SetRcptTosList(tos ...string) *Trx {
	var rcptTos []*addr.RcptTo
	for _, to := range tos {
		rcptTos = append(rcptTos, addr.NewRcptTo(to, "", "smtp"))
	}
	t.SetRcptTos(rcptTos)
	return t
}

func (t *Trx) HasRcptTo(rcptTo string) bool {
	return rcptto.Has(t.rcptTos, rcptTo)
}

func (t *Trx) AddRcptTo(rcptTo string, esmtpArgs string) {
	t.rcptTos = rcptto.Add(t.rcptTos, rcptTo, esmtpArgs)
}

func (t *Trx) DelRcptTo(rcptTo string) {
	t.rcptTos = rcptto.Del(t.rcptTos, rcptTo)
}

func (t *Trx) Headers() header2.Header {
	return t.header
}

func (t *Trx) SetHeaders(headers header2.Header) *Trx {
	r, err := io.ReadAll(headers.Reader())
	if err != nil {
		panic(err)
	}
	return t.SetHeadersRaw(r)
}

func (t *Trx) SetHeadersRaw(raw []byte) *Trx {
	canonicalRaw, _, err := transform.Bytes(&milterutil.CrLfCanonicalizationTransformer{}, raw)
	if err != nil {
		panic(err)
	}
	h, err := header.New(canonicalRaw)
	if err != nil {
		panic(err)
	}
	t.header = h
	t.origHeader = h.Copy()
	return t
}

func (t *Trx) Body() io.ReadSeeker {
	if t.body != nil {
		_, _ = t.body.Seek(0, io.SeekStart)
	}
	return t.body
}

func (t *Trx) SetBody(body io.ReadSeeker) *Trx {
	t.body = body
	return t
}

func (t *Trx) SetBodyBytes(b []byte) *Trx {
	t.SetBody(bytes.NewReader(b))
	return t
}

func (t *Trx) ReplaceBody(r io.Reader) {
	t.bodyReplacement = r
}

func (t *Trx) QueueId() string {
	return t.queueId
}

func (t *Trx) SetQueueId(value string) *Trx {
	t.queueId = value
	return t
}

func (t *Trx) Log(format string, v ...any) {
	t.logs = append(t.logs, fmt.Sprintf(format, v...))
}

func (t *Trx) Logs() []string {
	return t.logs
}

func (t *Trx) Modifications() []Modification {
	var mods []Modification
	if t.origMailFrom.Addr != t.mailFrom.Addr || t.origMailFrom.Args != t.mailFrom.Args {
		mods = append(mods, Modification{Kind: ChangeFrom, Addr: t.mailFrom.Addr, Args: t.mailFrom.Args})
	}
	deletions, additions := rcptto.Diff(t.origRcptTos, t.rcptTos)
	for _, r := range deletions {
		mods = append(mods, Modification{Kind: DelRcptTo, Addr: r.Addr})
	}
	for _, r := range additions {
		mods = append(mods, Modification{Kind: AddRcptTo, Addr: r.Addr, Args: r.Args})
	}
	changeInsertOps, addOps := header.Diff(t.origHeader, t.header)
	// apply change/insert operations in reverse for the indexes to be correct
	for i := len(changeInsertOps) - 1; i > -1; i-- {
		op := changeInsertOps[i]
		if op.Kind == header.KindInsert {
			mods = append(mods, Modification{Kind: InsertHeader, Index: op.Index, Name: op.Name, Value: op.Value})
		} else {
			mods = append(mods, Modification{Kind: ChangeHeader, Index: op.Index, Name: op.Name, Value: op.Value})
		}
	}
	for _, op := range addOps {
		mods = append(mods, Modification{Kind: InsertHeader, Index: op.Index + len(changeInsertOps) + 100, Name: op.Name, Value: op.Value})
	}

	if t.bodyReplacement != nil {
		b, err := io.ReadAll(t.bodyReplacement)
		if err != nil {
			panic(err)
		}
		mods = append(mods, Modification{Kind: ReplaceBody, Body: b})
	}
	return mods
}

var _ mailfilter.Trx = (*Trx)(nil)
