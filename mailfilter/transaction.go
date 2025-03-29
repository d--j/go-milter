package mailfilter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"regexp"

	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/internal/body"
	"github.com/d--j/go-milter/internal/header"
	"github.com/d--j/go-milter/internal/rcptto"
	"github.com/d--j/go-milter/mailfilter/addr"
	header2 "github.com/d--j/go-milter/mailfilter/header"
)

type MTA struct {
	Version string // value of [milter.MacroMTAVersion] macro
	FQDN    string // value of [milter.MacroMTAFQDN] macro
	Daemon  string // value of [milter.MacroDaemonName] macro
}

var sendmailVersionRe = regexp.MustCompile("^8\\.\\d+\\.\\d+\\b")

// IsSendmail returns true when [MTA.Version] looks like a Sendmail version number
func (m *MTA) IsSendmail() bool {
	return sendmailVersionRe.MatchString(m.Version)
}

type Connect struct {
	Host   string // The host name the MTA figured out for the remote client.
	Family string // "unknown", "unix", "tcp4" or "tcp6"
	Port   uint16 // If Family is "tcp4" or "tcp6" the remote port of client connecting to the MTA
	Addr   string // If Family "unix" the path to the unix socket. If "tcp4" or "tcp6" the IPv4 or IPv6 address of the remote client connecting to the MTA
	IfName string // The Name of the network interface the MTA connection was accepted at. Might be empty.
	IfAddr string // The IP address of the network interface the MTA connection was accepted at. Might be empty.
}

type Helo struct {
	Name        string // The HELO/EHLO hostname the client provided
	TlsVersion  string // TLSv1.3, TLSv1.2, ... or empty when no STARTTLS was used. Might even be empty when STARTTLS was used (when the MTA does not support the corresponding macro – almost all do).
	Cipher      string // The Cipher that client and MTA negotiated.
	CipherBits  string // The bits of the cipher used. E.g. 256. Might be "RSA equivalent" bits for e.g. elliptic curve ciphers.
	CertSubject string // If MutualTLS was used for the connection between client and MTA this holds the subject of the validated client certificate.
	CertIssuer  string // If MutualTLS was used for the connection between client and MTA this holds the subject of the issuer of the client certificate (CA or Sub-CA).
}

// transaction can be used to examine the data of the current mail transaction and
// also send changes to the message back to the MTA.
type transaction struct {
	mta                 MTA
	connect             Connect
	helo                Helo
	mailFrom            addr.MailFrom
	origMailFrom        addr.MailFrom
	rcptTos             []*addr.RcptTo
	origRcptTos         []*addr.RcptTo
	headers             *header.Header
	origHeaders         *header.Header
	enforceHeaderOrder  bool
	body                *body.Body
	bodyReplacement     io.Reader
	bufferedReplacement *body.Body
	queueId             string
	hasDecision         bool
	decision            Decision
	decisionErr         error
	quarantineReason    *string
	bodyOpt             bodyOption
}

func (t *transaction) MTA() *MTA {
	return &t.mta
}

func (t *transaction) Connect() *Connect {
	return &t.connect
}

func (t *transaction) Helo() *Helo {
	return &t.helo
}

func (t *transaction) QueueId() string {
	return t.queueId
}

func (t *transaction) cleanup() {
	t.headers = nil
	t.origHeaders = nil
	t.rcptTos = nil
	t.origRcptTos = nil
	t.quarantineReason = nil
	t.closeReplacementBody()
	if t.body != nil {
		_ = t.body.Close()
		t.body = nil
	}
}

func (t *transaction) response() *milter.Response {
	switch t.decision {
	case Accept:
		return milter.RespAccept
	case TempFail:
		return milter.RespTempFail
	case Reject:
		return milter.RespReject
	case Discard:
		return milter.RespDiscard
	default:
		if t.decision == nil {
			milter.LogWarning("milter: mailfilter returned unexpected <nil> Decision")
			return milter.RespTempFail
		}
		custom, ok := t.decision.(*customResponse)
		if !ok {
			milter.LogWarning("milter: mailfilter returned unexpected Decision of type %s: %+v", reflect.TypeOf(t.decision).String(), t.decision)
			return milter.RespTempFail
		}
		resp, err := milter.RejectWithCodeAndReason(custom.code, custom.reason)
		if err != nil {
			milter.LogWarning("milter: reject with custom reason failed, temp-fail instead: %s", err)
			return milter.RespTempFail
		}
		return resp
	}
}

func (t *transaction) makeDecision(ctx context.Context, decide DecisionModificationFunc) {
	if t.hasDecision {
		panic("calling makeDecision on a transaction that already has made a decision")
	}
	// make copies of data that user can change
	t.mailFrom = *t.origMailFrom.Copy()
	t.rcptTos = make([]*addr.RcptTo, len(t.origRcptTos))
	for i, r := range t.origRcptTos {
		t.rcptTos[i] = r.Copy()
	}
	if t.origHeaders != nil {
		t.headers = t.origHeaders.Copy()
	} else {
		t.origHeaders = &header.Header{}
		t.headers = &header.Header{}
	}
	// call the decider
	d, err := decide(ctx, t)
	// save decision
	t.hasDecision = true
	// if QuarantineResponse was used, replace it with Accept and record the reason,
	// so we can later send a quarantine modification action
	if qR, ok := d.(*quarantineResponse); ok {
		t.quarantineReason = &qR.reason
		d = Accept
	}
	t.decision = d
	t.decisionErr = err
}

// hasModifications checks quickly if there are any modifications - it does not actually compute them
func (t *transaction) hasModifications() bool {
	if !t.hasDecision {
		return false
	}
	// if we reject the message, we do not need to check for modifications
	if t.decision != Accept {
		return false
	}
	if t.quarantineReason != nil {
		return true
	}
	if t.origMailFrom.Addr != t.mailFrom.Addr || t.origMailFrom.Args != t.mailFrom.Args {
		return true
	}
	if t.bodyReplacement != nil {
		return true
	}
	if len(t.origRcptTos) != len(t.rcptTos) {
		return true
	}
	for i, r := range t.origRcptTos { // might give false positives because order does not matter
		if r.Addr != t.rcptTos[i].Addr || r.Args != t.rcptTos[i].Args {
			return true
		}
	}
	origFields := t.origHeaders.Fields()
	changedFields := t.headers.Fields()
	if origFields.Len() != changedFields.Len() {
		return true
	}
	for origFields.Next() && changedFields.Next() {
		if !bytes.Equal(origFields.Raw(), changedFields.Raw()) {
			return true
		}
	}
	return false
}

func (t *transaction) sendModifications(m *milter.Modifier) error {
	defer func() {
		t.closeReplacementBody()
	}()
	// if we reject the message, we do not need to send modifications
	// they are useless since we do not keep the message anyway
	if t.decision != Accept {
		return nil
	}
	if t.origMailFrom.Addr != t.mailFrom.Addr || t.origMailFrom.Args != t.mailFrom.Args {
		if err := m.ChangeFrom(t.mailFrom.Addr, t.mailFrom.Args); err != nil {
			return err
		}
	}
	deletions, additions := rcptto.Diff(t.origRcptTos, t.rcptTos)
	for _, r := range deletions {
		if err := m.DeleteRecipient(r.Addr); err != nil {
			return err
		}
	}
	for _, r := range additions {
		if err := m.AddRecipient(r.Addr, r.Args); err != nil {
			return err
		}
	}
	changeInsertOps, addOps := header.DiffOrRecreate(t.enforceHeaderOrder, t.origHeaders, t.headers)
	// apply change/insert operations in reverse for the indexes to be correct
	for i := len(changeInsertOps) - 1; i > -1; i-- {
		op := changeInsertOps[i]
		if op.Kind == header.KindInsert {
			if err := m.InsertHeader(op.Index, op.Name, op.Value); err != nil {
				return err
			}
		} else {
			if err := m.ChangeHeader(op.Index, op.Name, op.Value); err != nil {
				return err
			}
		}
	}
	for _, op := range addOps {
		// Sendmail has headers in its envelop headers list that it does not send to the milter.
		// But they *do* count to the insert index?! So for sendmail we cannot really add a header at a specific position.
		// (Other than beginning, that is index 0).
		// We add the arbitrary number 100 to the index so that we skip any and all "hidden" sendmail headers when we
		// want to insert at the end of the header list.
		// We do not use m.AddHeader since that also is not guaranteed to add the header at the end…
		if err := m.InsertHeader(op.Index+len(changeInsertOps)+100, op.Name, op.Value); err != nil {
			return err
		}
	}
	if t.bufferedReplacement != nil {
		if err := m.ReplaceBody(t.bufferedReplacement); err != nil {
			return err
		}
	} else if t.bodyReplacement != nil {
		if err := m.ReplaceBody(t.bodyReplacement); err != nil {
			return err
		}
	}
	if t.quarantineReason != nil {
		if err := m.Quarantine(*t.quarantineReason); err != nil {
			return err
		}
	}
	return nil
}

func (t *transaction) addHeader(key string, raw []byte) {
	if t.origHeaders == nil {
		t.origHeaders = &header.Header{}
	}
	t.origHeaders.AddRaw(key, raw)
}

func (t *transaction) addBodyChunk(chunk []byte) (err error) {
	if t.body == nil {
		t.body = body.New(t.bodyOpt.MaxMem, t.bodyOpt.MaxSize)
	}
	_, err = t.body.Write(chunk)
	if errors.Is(err, body.ErrBodyTooLarge) {
		if t.bodyOpt.MaxAction == RejectMessageWhenTooBig {
			return err
		}
		err = nil
		if t.bodyOpt.MaxAction == ClearWhenTooBig {
			t.body = body.New(1024, 0)
		}
		t.body.DisableWriting = true
	}
	return
}

func (t *transaction) MailFrom() *addr.MailFrom {
	return &t.mailFrom
}

func (t *transaction) ChangeMailFrom(from string, esmtpArgs string) {
	t.mailFrom.Addr = from
	t.mailFrom.Args = esmtpArgs
}

func (t *transaction) RcptTos() []*addr.RcptTo {
	return t.rcptTos
}

func (t *transaction) HasRcptTo(rcptTo string) bool {
	return rcptto.Has(t.rcptTos, rcptTo)
}

func (t *transaction) AddRcptTo(rcptTo string, esmtpArgs string) {
	t.rcptTos = rcptto.Add(t.rcptTos, rcptTo, esmtpArgs)
}

func (t *transaction) DelRcptTo(rcptTo string) {
	t.rcptTos = rcptto.Del(t.rcptTos, rcptTo)
}

func (t *transaction) Headers() header2.Header {
	return t.headers
}

func (t *transaction) HeadersEnforceOrder() {
	if t.mta.IsSendmail() {
		t.enforceHeaderOrder = true
	}
}

func (t *transaction) Body() io.ReadSeeker {
	if t.body == nil {
		return nil
	}
	_, _ = t.body.Seek(0, io.SeekStart)
	return t.body
}

func (t *transaction) ReplaceBody(r io.Reader) {
	t.closeReplacementBody()
	t.bodyReplacement = r
}

func (t *transaction) Data() io.Reader {
	if t.bodyReplacement != nil && t.bufferedReplacement == nil {
		t.bufferedReplacement = body.New(t.bodyOpt.MaxMem, 0)
		_, err := io.Copy(t.bufferedReplacement, t.bodyReplacement)
		if err != nil {
			t.bufferedReplacement = nil
			return io.MultiReader(t.Headers().Reader(), body.ErrReader{Err: err})
		}
	}
	if t.bufferedReplacement != nil {
		_, err := t.bufferedReplacement.Seek(0, io.SeekStart)
		if err != nil {
			return io.MultiReader(t.Headers().Reader(), body.ErrReader{Err: err})
		}
		return io.MultiReader(t.Headers().Reader(), t.bufferedReplacement)
	}
	b := t.Body()
	if b != nil {
		return io.MultiReader(t.Headers().Reader(), b)
	}
	return t.Headers().Reader()
}

func (t *transaction) closeReplacementBody() {
	if t.bodyReplacement != nil {
		if closer, ok := t.bodyReplacement.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				milter.LogWarning("error while closing replacement body: %s", err)
			}
		}
		t.bodyReplacement = nil
		t.bufferedReplacement = nil
	}
}

func (t *transaction) wantsBodyChunks() bool {
	if t.hasDecision || t.bodyOpt.Skip {
		return false
	}
	if t.body != nil && t.body.DisableWriting {
		return false
	}
	return true
}

var _ Trx = (*transaction)(nil)
