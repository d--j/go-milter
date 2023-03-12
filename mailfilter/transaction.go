package mailfilter

import (
	"context"
	"io"
	"os"

	"github.com/d--j/go-milter"
)

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

// Transaction can be used to examine the data of the current mail transaction and
// also send changes to the message back to the MTA.
type Transaction struct {
	// Connect holds the [Connect] information of this transaction.
	Connect Connect

	// Helo holds the [Helo] information of this transaction.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtConnect].
	Helo Helo

	// MailFrom holds the [MailFrom] of this transaction.
	// You can change this and your changes get send back to the MTA.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtHelo].
	MailFrom MailFrom

	// RcptTos holds the [RcptTo] recipient slice of this transaction.
	// You can change this and your changes get send back to the MTA.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtMailFrom].
	RcptTos []RcptTo

	// QueueId is the queue ID the MTA assigned for this transaction.
	// You cannot change this value.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtMailFrom].
	QueueId string

	// Headers are the [Header] fields of this message.
	// You can use methods of this to change the header fields of the current message.
	//
	// Do not replace this variable. Always use the modification methods of [Header] and [Header.Fields].
	// The mail filter might panic if you do replace Headers.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtData].
	Headers *Header

	hasDecision      bool
	decision         Decision
	decisionErr      error
	headers          *Header
	body             *os.File
	mailFrom         MailFrom
	rcptTos          []RcptTo
	replacementBody  io.Reader
	quarantineReason *string
}

func (t *Transaction) cleanup() {
	t.Headers = nil
	t.headers = nil
	t.RcptTos = nil
	t.rcptTos = nil
	t.quarantineReason = nil
	t.closeReplacementBody()
	if t.body != nil {
		_ = t.body.Close()
		_ = os.Remove(t.body.Name())
		t.body = nil
	}
}

func (t *Transaction) response() *milter.Response {
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
		resp, err := milter.RejectWithCodeAndReason(t.decision.getCode(), t.decision.getReason())
		if err != nil {
			milter.LogWarning("milter: reject with custom reason failed, temp-fail instead: %s", err)
			return milter.RespTempFail
		}
		return resp
	}
}

func (t *Transaction) makeDecision(ctx context.Context, decide DecisionModificationFunc) {
	if t.hasDecision {
		panic("calling makeDecision on a Transaction that already has made a decision")
	}
	// make copies of data that user can change
	t.MailFrom = t.mailFrom
	t.RcptTos = make([]RcptTo, len(t.rcptTos))
	for i, r := range t.rcptTos {
		t.RcptTos[i] = r
	}
	if t.headers != nil {
		t.Headers = t.headers.copy()
	} else {
		t.headers = &Header{}
		t.Headers = &Header{}
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
func (t *Transaction) hasModifications() bool {
	if !t.hasDecision {
		return false
	}
	if t.quarantineReason != nil {
		return true
	}
	if t.mailFrom.Addr != t.MailFrom.Addr || t.mailFrom.Args != t.MailFrom.Args {
		return true
	}
	if t.replacementBody != nil {
		return true
	}
	if len(t.rcptTos) != len(t.RcptTos) {
		return true
	}
	for i, r := range t.rcptTos { // might give false positives because order does not matter
		if r.Addr != t.RcptTos[i].Addr || r.Args != t.RcptTos[i].Args {
			return true
		}
	}
	origFields := t.headers.Fields()
	changedFields := t.Headers.Fields()
	if origFields.Len() != changedFields.Len() {
		return true
	}
	for origFields.Next() && changedFields.Next() {
		if origFields.raw() != changedFields.raw() {
			return true
		}
	}
	return false
}

func (t *Transaction) sendModifications(m *milter.Modifier) error {
	if t.mailFrom.Addr != t.MailFrom.Addr || t.mailFrom.Args != t.MailFrom.Args {
		if err := m.ChangeFrom(t.MailFrom.Addr, t.MailFrom.Args); err != nil {
			return err
		}
	}
	deletions, additions := calculateRcptToDiff(t.rcptTos, t.RcptTos)
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
	changeInsertOps, addOps := calculateHeaderModifications(t.headers, t.Headers)
	// apply change/insert operations in reverse for the indexes to be correct
	for i := len(changeInsertOps) - 1; i > -1; i-- {
		op := changeInsertOps[i]
		if op.Kind == kindInsert {
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
		// But the *do* count to the insert index?! So for sendmail we cannot really add a header at a specific position.
		// (Other than beginning, that is index 0).
		// We add the arbitrary number 100 to the index so that we skip any and all "hidden" sendmail headers when we
		// want to insert at the end of the header list.
		// We do not use m.AddHeader since that also is not guaranteed to add the header at the end…
		if err := m.InsertHeader(op.Index+len(changeInsertOps)+100, op.Name, op.Value); err != nil {
			return err
		}
	}
	if t.replacementBody != nil {
		defer func() {
			t.closeReplacementBody()
		}()
		if err := m.ReplaceBody(t.replacementBody); err != nil {
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

func (t *Transaction) addHeader(key string, raw string) {
	if t.headers == nil {
		t.headers = &Header{}
	}
	t.headers.addRaw(key, raw)
}

func (t *Transaction) addBodyChunk(chunk []byte) (err error) {
	if t.body == nil {
		t.body, err = os.CreateTemp("", "body-*")
		if err != nil {
			return
		}
	}
	_, err = t.body.Write(chunk)
	return
}

// HasRcptTo returns true when rcptTo is in the list of recipients.
//
// rcptTo gets compared to the existing recipients IDNA address aware.
func (t *Transaction) HasRcptTo(rcptTo string) bool {
	findR := RcptTo{
		addr:      addr{Addr: rcptTo, Args: ""},
		transport: "",
	}
	findLocal, findDomain := findR.Local(), findR.AsciiDomain()
	for _, r := range t.RcptTos {
		if r.Local() == findLocal && r.AsciiDomain() == findDomain {
			return true
		}
	}
	return false
}

// AddRcptTo adds the rcptTo (without angles) to the list of recipients with the ESMTP arguments esmtpArgs.
// If rcptTo is already in the list of recipients only the esmtpArgs of this recipient get updated.
//
// rcptTo gets compared to the existing recipients IDNA address aware.
func (t *Transaction) AddRcptTo(rcptTo string, esmtpArgs string) {
	addR := RcptTo{
		addr:      addr{Addr: rcptTo, Args: esmtpArgs},
		transport: "smtp",
	}
	findLocal, findDomain := addR.Local(), addR.AsciiDomain()
	for i, r := range t.RcptTos {
		if r.Local() == findLocal && r.AsciiDomain() == findDomain {
			t.RcptTos[i].Args = esmtpArgs
			return
		}
	}
	t.RcptTos = append(t.RcptTos, addR)
}

// DelRcptTo deletes the rcptTo (without angles) from the list of recipients.
//
// rcptTo gets compared to the existing recipients IDNA address aware.
func (t *Transaction) DelRcptTo(rcptTo string) {
	findR := RcptTo{
		addr:      addr{Addr: rcptTo, Args: ""},
		transport: "",
	}
	findLocal, findDomain := findR.Local(), findR.AsciiDomain()
	for i, r := range t.RcptTos {
		if r.Local() == findLocal && r.AsciiDomain() == findDomain {
			t.RcptTos = append(t.RcptTos[:i], t.RcptTos[i+1:]...)
			return
		}
	}
}

// Body gets you a [io.ReadSeeker] of the body. The reader seeked to the start of the body.
//
// This method returns nil when you used [WithDecisionAt] with anything other than [DecisionAtEndOfMessage]
// or you used [WithoutBody].
func (t *Transaction) Body() io.ReadSeeker {
	if t.body == nil {
		return nil
	}
	_, _ = t.body.Seek(0, io.SeekStart)
	return t.body
}

// ReplaceBody replaces the body of the current message with the contents
// of the [io.Reader] r.
func (t *Transaction) ReplaceBody(r io.Reader) {
	t.closeReplacementBody()
	t.replacementBody = r
}

func (t *Transaction) closeReplacementBody() {
	if t.replacementBody != nil {
		if closer, ok := t.replacementBody.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				milter.LogWarning("error while closing replacement body: %s", err)
			}
		}
		t.replacementBody = nil
	}
}
