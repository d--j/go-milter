// Package testtrx can be used to test mailfilter based filter functions
package testtrx

import (
	"bytes"
	"github.com/d--j/go-milter/internal/body"
	"github.com/d--j/go-milter/internal/header"
	"github.com/d--j/go-milter/internal/rcptto"
	"github.com/d--j/go-milter/mailfilter"
	"github.com/d--j/go-milter/mailfilter/addr"
	header2 "github.com/d--j/go-milter/mailfilter/header"
	"github.com/d--j/go-milter/milterutil"
	"golang.org/x/text/transform"
	"io"
)

// Trx implements [mailfilter.Trx] for unit tests.
// Use this struct when you want to test your decision functions.
// You can use the fluent Set* methods of this struct to build up the transaction you want to test.
// After you passed the Trx to your decision function, you can call [Trx.Modifications] to
// check that your decision function did what was expected of it.
type Trx struct {
	mta                mailfilter.MTA
	connect            mailfilter.Connect
	helo               mailfilter.Helo
	mailFrom           addr.MailFrom
	origMailFrom       addr.MailFrom
	rcptTos            []*addr.RcptTo
	origRcptTos        []*addr.RcptTo
	queueId            string
	header             *header.Header
	origHeader         *header.Header
	enforceHeaderOrder bool
	body               io.ReadSeeker
	bodyReplacement    io.Reader
	replacementBuffer  *body.Body
	// MaxMem can be used to set the maximum memory used for the body replacement buffer.
	// If MaxMem is nil, the default value of 200KiB is used.
	// If the body replacement buffer exceeds this limit, the replacement buffer will automatically be buffered to disk.
	MaxMem *int
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

func (t *Trx) HeadersEnforceOrder() {
	if t.mta.IsSendmail() {
		t.enforceHeaderOrder = true
	}
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

func (t *Trx) Data() io.Reader {
	if t.bodyReplacement != nil && t.replacementBuffer == nil {
		maxMem := 200 * 1024
		if t.MaxMem != nil {
			maxMem = *t.MaxMem
		}
		t.replacementBuffer = body.New(maxMem, 0)
		_, err := io.Copy(t.replacementBuffer, t.bodyReplacement)
		if err != nil {
			t.replacementBuffer = nil
			return io.MultiReader(t.Headers().Reader(), body.ErrReader{Err: err})
		}
	}
	if t.replacementBuffer != nil {
		_, err := t.replacementBuffer.Seek(0, io.SeekStart)
		if err != nil {
			return io.MultiReader(t.Headers().Reader(), body.ErrReader{Err: err})
		}
		return io.MultiReader(t.Headers().Reader(), t.replacementBuffer)
	}
	b := t.Body()
	if b != nil {
		return io.MultiReader(t.Headers().Reader(), b)
	}
	return t.Headers().Reader()
}

func (t *Trx) QueueId() string {
	return t.queueId
}

func (t *Trx) SetQueueId(value string) *Trx {
	t.queueId = value
	return t
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
	changeInsertOps, addOps := header.DiffOrRecreate(t.enforceHeaderOrder, t.origHeader, t.header)
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

	if t.replacementBuffer != nil {
		_, err := t.replacementBuffer.Seek(0, io.SeekStart)
		if err != nil {
			panic(err)
		}
		b, err := io.ReadAll(t.replacementBuffer)
		if err != nil {
			panic(err)
		}
		mods = append(mods, Modification{Kind: ReplaceBody, Body: b})
	} else if t.bodyReplacement != nil {
		b, err := io.ReadAll(t.bodyReplacement)
		if err != nil {
			panic(err)
		}
		mods = append(mods, Modification{Kind: ReplaceBody, Body: b})
	}
	return mods
}

var _ mailfilter.Trx = (*Trx)(nil)
