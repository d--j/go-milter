package milter

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/d--j/go-milter/internal/wire"
	"github.com/d--j/go-milter/milterutil"
	"io"
	"math"
	"net/textproto"
	"strings"
)

type ActionType int

const (
	ActionAccept ActionType = iota + 1
	ActionContinue
	ActionDiscard
	ActionReject
	ActionTempFail
	ActionSkip
	ActionRejectWithCode
)

// Action represents the action that the milter wants to take on the current message.
// The client can call StopProcessing on it to check if the milter wants to abort the connection/message.
type Action struct {
	Type ActionType

	// SMTP code if milter wants to abort the connection/message. Zero otherwise.
	SMTPCode uint16
	// Properly formatted reply text if milter wants to abort the connection/message. Empty string otherwise.
	SMTPReply string
}

// StopProcessing returns true when the milter wants to immediately stop this SMTP connection or reject this recipient.
// (a.Type is one of ActionReject, ActionTempFail or ActionRejectWithCode).
// You can use [Action.SMTPReply] to send as reply to the current SMTP command.
func (a Action) StopProcessing() bool {
	switch a.Type {
	case ActionReject, ActionTempFail, ActionRejectWithCode:
		return true
	default:
		return false
	}
}

func (a Action) String() string {
	switch a.Type {
	case ActionAccept:
		return "Accept"
	case ActionContinue:
		return "Continue"
	case ActionDiscard:
		return "Discard"
	case ActionReject:
		return fmt.Sprintf("Reject %d %q", a.SMTPCode, a.SMTPReply)
	case ActionTempFail:
		return fmt.Sprintf("TempFail %d %q", a.SMTPCode, a.SMTPReply)
	case ActionSkip:
		return "Skip"
	case ActionRejectWithCode:
		return fmt.Sprintf("RejectWithCode %d %q", a.SMTPCode, a.SMTPReply)
	default:
		return fmt.Sprintf("Unknown action %d", a.Type)
	}
}

func parseAction(msg *wire.Message) (*Action, error) {
	act := &Action{SMTPCode: 250, SMTPReply: "250 accept"}

	switch wire.ActionCode(msg.Code) {
	case wire.ActAccept:
		act.Type = ActionAccept
	case wire.ActContinue:
		act.Type = ActionContinue
	case wire.ActDiscard:
		act.Type = ActionDiscard
	case wire.ActReject:
		act.Type = ActionReject
		act.SMTPCode = 550
		act.SMTPReply = "550 5.7.1 Command rejected"
	case wire.ActTempFail:
		act.Type = ActionTempFail
		act.SMTPCode = 451
		act.SMTPReply = "451 4.7.1 Service unavailable - try again later"
	case wire.ActSkip:
		act.Type = ActionSkip
	case wire.ActReplyCode:
		if len(msg.Data) <= 4 {
			return nil, fmt.Errorf("action read: unexpected data length: %d", len(msg.Data))
		}
		if msg.Data[len(msg.Data)-1] != 0 {
			return nil, fmt.Errorf("action read: missing NUL terminator")
		}
		cmd := msg.Data[:len(msg.Data)-1]
		checker := textproto.NewReader(bufio.NewReader(bytes.NewReader(cmd)))
		// this also accepts FTP style multi-line responses as valid
		// It's highly unlikely that milter sends one of those, so we ignore this false positive
		code, _, err := checker.ReadResponse(0)
		if err != nil {
			return nil, fmt.Errorf("action read: malformed SMTP response: %q", msg.Data)
		}
		if code < 400 || code > 599 {
			return nil, fmt.Errorf("action read: invalid SMTP code: %d", code)
		}
		act.Type = ActionRejectWithCode
		act.SMTPCode = uint16(code)
		act.SMTPReply = strings.TrimRight(wire.ReadCString(msg.Data), "\r\n") // use raw response as it was formatted by milter
	default:
		return nil, fmt.Errorf("action read: unexpected code: %c", msg.Code)
	}

	return act, nil
}

type ModifyActionType int

const (
	ActionAddRcpt ModifyActionType = iota + 1
	ActionDelRcpt
	ActionQuarantine
	ActionReplaceBody
	ActionChangeFrom
	ActionAddHeader
	ActionChangeHeader
	ActionInsertHeader
)

type ModifyAction struct {
	Type ModifyActionType

	// Recipient to add/remove if Type == ActionAddRcpt or ActionDelRcpt.
	// This value already includes the necessary <>.
	Rcpt string

	// ESMTP arguments for recipient address if Type = ActionAddRcpt.
	RcptArgs string

	// New envelope sender if Type = ActionChangeFrom.
	// This value already includes the necessary <>.
	From string

	// ESMTP arguments for envelope sender if Type = ActionChangeFrom.
	FromArgs string

	// Portion of body to be replaced if Type == ActionReplaceBody.
	Body []byte

	// Index of the header field to be changed if Type = ActionChangeHeader or Type = ActionInsertHeader.
	// Index is 1-based.
	//
	// If Type = ActionChangeHeader the index is per canonical value of HdrName.
	// E.g. HeaderIndex = 3 and HdrName = "DKIM-Signature" means "change third field with the canonical header name Dkim-Signature".
	// Order is the same as of HeaderField calls.
	//
	// If Type = ActionInsertHeader the index is global to all headers, 1-based and means "insert after the HeaderIndex header".
	// A HeaderIndex of 0 has the special meaning "at the very beginning".
	//
	// Deleted headers (Type = ActionChangeHeader and HeaderValue == "") may change the indexes of the other headers.
	// Postfix MTA removes the header from the linked list (and thus change the indexes of headers coming after the deleted header).
	// Sendmail on the other hand will only mark the header as deleted.
	// To be consistent, you should delete headers in reverse order.
	HeaderIndex uint32

	// Header field name to be added/changed if Type == ActionAddHeader or
	// ActionChangeHeader or ActionInsertHeader.
	HeaderName string

	// Header field value to be added/changed if Type == ActionAddHeader or
	// ActionChangeHeader or ActionInsertHeader. If set to empty string - the field
	// should be removed.
	HeaderValue string

	// Quarantine reason if Type == ActionQuarantine.
	Reason string
}

func (ma ModifyAction) String() string {
	switch ma.Type {
	case ActionAddRcpt:
		return fmt.Sprintf("AddRcpt %q %q", ma.Rcpt, ma.RcptArgs)
	case ActionDelRcpt:
		return fmt.Sprintf("DelRcpt %q", ma.Rcpt)
	case ActionChangeFrom:
		return fmt.Sprintf("ChangeFrom %q %q", ma.From, ma.FromArgs)
	case ActionQuarantine:
		return fmt.Sprintf("Quarantine %q", ma.Reason)
	case ActionReplaceBody:
		bin := sha1.Sum(ma.Body)
		hash := hex.EncodeToString(bin[:])
		return fmt.Sprintf("ReplaceBody len(body) = %d sha1(body) = %s", len(ma.Body), hash)
	case ActionAddHeader:
		return fmt.Sprintf("AddHeader %q %q", ma.HeaderName, ma.HeaderValue)
	case ActionChangeHeader:
		return fmt.Sprintf("ChangeHeader %d %q %q", ma.HeaderIndex, ma.HeaderName, ma.HeaderValue)
	case ActionInsertHeader:
		return fmt.Sprintf("InsertHeader %d %q %q", ma.HeaderIndex, ma.HeaderName, ma.HeaderValue)
	default:
		return fmt.Sprintf("Unknown modify action %d", ma.Type)
	}
}

func parseModifyAct(msg *wire.Message) (*ModifyAction, error) {
	act := &ModifyAction{}
	data := msg.Data
	switch wire.ModifyActCode(msg.Code) {
	case wire.ActAddRcpt:
		argv := bytes.Split(data, []byte{0x00})
		if len(argv) != 2 {
			return nil, fmt.Errorf("read modify action: wrong number of arguments %d for ActAddRcpt action", len(argv))
		}
		act.Type = ActionAddRcpt
		act.Rcpt = string(argv[0])
	case wire.ActAddRcptPar:
		argv := bytes.Split(data, []byte{0x00})
		if len(argv) > 3 || len(argv) < 2 {
			return nil, fmt.Errorf("read modify action: wrong number of arguments %d for ActAddRcpt action", len(argv))
		}
		act.Type = ActionAddRcpt
		act.Rcpt = string(argv[0])
		if len(argv) == 3 {
			act.RcptArgs = string(argv[1])
		}
	case wire.ActDelRcpt:
		if len(data) == 0 || data[len(data)-1] != 0 {
			return nil, fmt.Errorf("action read: missing NUL terminator")
		}
		act.Type = ActionDelRcpt
		act.Rcpt = wire.ReadCString(data)
	case wire.ActQuarantine:
		if len(data) == 0 || data[len(data)-1] != 0 {
			return nil, fmt.Errorf("action read: missing NUL terminator")
		}
		act.Type = ActionQuarantine
		act.Reason = wire.ReadCString(data)
	case wire.ActReplBody:
		act.Type = ActionReplaceBody
		act.Body = data
	case wire.ActChangeFrom:
		argv := bytes.Split(data, []byte{0x00})
		if len(argv) > 3 || len(argv) < 2 {
			return nil, fmt.Errorf("read modify action: wrong number of arguments %d for ActChangeFrom action", len(argv))
		}
		act.Type = ActionChangeFrom
		act.From = string(argv[0])
		if len(argv) == 3 {
			act.FromArgs = string(argv[1])
		}
	case wire.ActChangeHeader, wire.ActInsertHeader:
		if len(data) < 4 {
			return nil, fmt.Errorf("read modify action: missing header index")
		}
		if wire.ModifyActCode(msg.Code) == wire.ActChangeHeader {
			act.Type = ActionChangeHeader
		} else {
			act.Type = ActionInsertHeader
		}
		act.HeaderIndex = binary.BigEndian.Uint32(data)

		// Sendmail 8 compatibility
		if wire.ModifyActCode(msg.Code) == wire.ActChangeHeader && act.HeaderIndex == 0 {
			act.HeaderIndex = 1
		}

		data = data[4:]
		fallthrough
	case wire.ActAddHeader:
		argv := bytes.Split(data, []byte{0x00})
		if len(argv) != 3 {
			return nil, fmt.Errorf("read modify action: wrong number of arguments %d for header action: %v", len(argv), argv)
		}
		if wire.ModifyActCode(msg.Code) == wire.ActAddHeader {
			act.Type = ActionAddHeader
		}
		act.HeaderName = string(argv[0])
		act.HeaderValue = string(argv[1])
	default:
		return nil, fmt.Errorf("read modify action: unexpected message code: %v", msg.Code)
	}

	return act, nil
}

func hasAngle(str string) bool {
	return len(str) > 1 && str[0] == '<' && str[len(str)-1] == '>'
}

// AddAngle adds <> to an address. If str already has <>, then str is returned unchanged.
func AddAngle(str string) string {
	if hasAngle(str) {
		return str
	} else {
		return fmt.Sprintf("<%s>", str)
	}
}

// RemoveAngle removes <> from an address. If str does not have <>, then str is returned unchanged.
func RemoveAngle(str string) string {
	if hasAngle(str) {
		return str[1 : len(str)-1]
	} else {
		return str
	}
}

// validName checks if the provided name is a valid header name.
func validName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, r := range []byte(name) {
		if r <= ' ' || r >= '\x7F' || r == ':' {
			return false
		}
	}
	return true
}

var ErrModificationNotAllowed = errors.New("milter: modification not allowed via milter protocol negotiation")
var ErrVersionTooLow = errors.New("milter: action not allowed in this milter protocol version")

// Modifier provides access to [Macros] to the callback handlers. It also defines a
// number of functions that can be used by callback handlers to modify processing of the email message.
// Besides [Modifier.Progress] they can only be called in the EndOfMessage callback.
type Modifier interface {
	Macros

	// Version returns the negotiated milter protocol version.
	Version() uint32
	// Protocol returns the negotiated milter protocol flags.
	Protocol() OptProtocol
	// Actions returns the negotiated milter actions flags.
	Actions() OptAction
	// MaxDataSize returns the maximum data size that the MTA will accept.
	// This is the value that was negotiated with the MTA.
	MaxDataSize() DataSize
	// MilterId returns an identifier of this Milter instance.
	// This is a unique, incrementing identifier in the realm of a single Server.
	MilterId() uint64

	// AddRecipient appends a new envelope recipient for current message.
	// You can optionally specify esmtpArgs to pass along. You need to negotiate this via [OptAddRcptWithArgs] with the MTA.
	//
	// Sendmail will validate the provided esmtpArgs and if it deems them invalid it will error out.
	AddRecipient(r string, esmtpArgs string) error
	// DeleteRecipient removes an envelope recipient address from message
	DeleteRecipient(r string) error
	// ReplaceBodyRawChunk sends one chunk of the body replacement.
	//
	// The chunk get send as-is. Caller needs to ensure that the chunk does not exceed the maximum configured data size (defaults to [DataSize64K])
	//
	// You should do the ReplaceBodyRawChunk calls all in one go without intersecting it with other modification actions.
	// MTAs like Postfix do not allow that.
	ReplaceBodyRawChunk(chunk []byte) error
	// ReplaceBody reads from r and send its contents in the least amount of chunks to the MTA.
	//
	// This function does not do any CR LF line ending canonicalization or maximum line length enforcements.
	// If you need that you can use the various transform.Transformers of the milterutil package to wrap your reader.
	//
	//	t := transform.Chain(&milterutil.CrLfCanonicalizationTransformer{}, &milterutil.MaximumLineLengthTransformer{})
	//	wrappedR := transform.NewReader(r, t)
	//	m.ReplaceBody(wrappedR)
	//
	// This function tries to use as few calls to [Modifier.ReplaceBodyRawChunk] as possible.
	//
	// You can call ReplaceBody multiple times. The MTA will combine all those calls into one message.
	//
	// You should do the ReplaceBody calls all in one go without intersecting it with other modification actions.
	// MTAs like Postfix do not allow that.
	ReplaceBody(r io.Reader) error
	// Quarantine a message by giving a reason to hold it. Only makes sense when you RespAccept the message.
	Quarantine(reason string) error
	// AddHeader appends a new email message header to the message
	//
	// Unfortunately when interacting with Sendmail it is not guaranteed that the header
	// will be added at the end. If Sendmail has a (maybe deleted) header of the same name
	// in the list of headers, this header will be altered/re-used instead of adding a new
	// header at the end.
	//
	// If you always want to add the header at the very end you need to use InsertHeader with
	// a very high index.
	//
	// The header name must be valid. It can only contain printable ASCII characters without SP and colon.
	//
	// value can include newlines. They will be canonicalized to LF.
	// If the value includes newlines, it should also have the continuation character (SP or HT) at the beginning of the lines.
	// The continuation character is not mandatory, but it is recommended to use it.
	// NUL characters get converted to SP.
	AddHeader(name, value string) error
	// ChangeHeader replaces the header at the specified position with a new one.
	// The index is per canonical header name and one-based. To delete a header pass an empty value.
	// If the index is bigger than there are headers with that name, then ChangeHeader will actually
	// add a new header at the end of the header list (With the same semantic as AddHeader).
	//
	// The header name must be valid. It can only contain printable ASCII characters without SP and colon.
	//
	// value can include newlines. They will be canonicalized to LF.
	// If the value includes newlines, it should also have the continuation character (SP or HT) at the beginning of the lines.
	// The continuation character is not mandatory, but it is recommended to use it.
	// NUL characters get converted to SP.
	ChangeHeader(index int, name, value string) error
	// InsertHeader inserts the header at the specified position.
	// index is one-based. The index 0 means at the very beginning.
	// If the index is bigger than the number of headers, then the header will be added at the end.
	// Unlike ChangeHeader, index is not per canonical header name but the index in the list of all headers.
	//
	// The header name must be valid. It can only contain printable ASCII characters without SP and colon.
	//
	// value can include newlines. They will be canonicalized to LF.
	// If the value includes newlines, it should also have the continuation character (SP or HT) at the beginning of the lines.
	// The continuation character is not mandatory, but it is recommended to use it.
	// NUL characters get converted to SP.
	//
	// Unfortunately when interacting with Sendmail the index is used to find the position
	// in Sendmail's internal list of headers. Not all of those internal headers get send to the milter.
	// Thus, you cannot really add a header at a specific position when the milter client is Sendmail.
	InsertHeader(index int, name, value string) error
	// ChangeFrom replaces the FROM envelope header with value.
	// You can also define ESMTP arguments. But beware of the following Sendmail comment:
	//
	//	Even though all ESMTP arguments could be set via this call,
	//	it does not make sense to do so for many of them,
	//	e.g., SIZE and BODY.
	//	Setting those may cause problems, proper care must be taken.
	//	Moreover, there is no feedback from the MTA to the milter
	//	whether the call was successful.
	ChangeFrom(value string, esmtpArgs string) error
	// Progress tells the client that there is progress in a long operation
	// and that the client should not time out the milter connection.
	//
	// This function is only available when the negotiated milter protocol version is >= 6.
	//
	// This function can be called in any callback handler (unlike all other functions of [Modifier]).
	// It will send a progress notification packet to the MTA.
	// When it returns an error besides ErrVersionTooLow, the connection to the MTA is broken.
	Progress() error
}

type modifierState int

const (
	modifierStateReadOnly modifierState = iota
	modifierStateProgressOnly
	modifierStateReadWrite
)

type modifier struct {
	macros      Macros
	state       modifierState
	writePacket func(*wire.Message) error
	version     uint32
	protocol    OptProtocol
	actions     OptAction
	maxDataSize DataSize
	milterId    uint64
}

func (m *modifier) Get(name MacroName) string {
	return m.macros.Get(name)
}

func (m *modifier) GetEx(name MacroName) (string, bool) {
	return m.macros.GetEx(name)
}

func (m *modifier) AddRecipient(r string, esmtpArgs string) error {
	if m.actions&OptAddRcpt == 0 && m.actions&OptAddRcptWithArgs == 0 {
		return ErrModificationNotAllowed
	}
	if esmtpArgs != "" && m.actions&OptAddRcptWithArgs == 0 {
		return ErrModificationNotAllowed
	}
	code := wire.ActAddRcpt
	var buffer bytes.Buffer
	buffer.WriteString(AddAngle(milterutil.NewlineToSpace(r)))
	buffer.WriteByte(0)
	// send wire.ActAddRcptPar when that is the only allowed action, or we need to send it because esmptArgs ist set
	if (esmtpArgs != "" && m.actions&OptAddRcptWithArgs != 0) || (esmtpArgs == "" && m.actions&OptAddRcpt == 0) {
		buffer.WriteString(milterutil.NewlineToSpace(esmtpArgs))
		buffer.WriteByte(0)
		code = wire.ActAddRcptPar
	}
	if code == wire.ActAddRcptPar && m.version < 6 {
		return ErrVersionTooLow
	}
	return m.write(modifierStateReadWrite, newResponse(wire.Code(code), buffer.Bytes()))
}

func (m *modifier) DeleteRecipient(r string) error {
	if m.actions&OptRemoveRcpt == 0 {
		return ErrModificationNotAllowed
	}
	resp, err := newResponseStr(wire.Code(wire.ActDelRcpt), AddAngle(milterutil.NewlineToSpace(r)))
	if err != nil {
		return err
	}
	return m.write(modifierStateReadWrite, resp)
}

func (m *modifier) ReplaceBodyRawChunk(chunk []byte) error {
	if m.actions&OptChangeBody == 0 {
		return ErrModificationNotAllowed
	}
	if len(chunk) > int(m.maxDataSize) {
		return fmt.Errorf("milter: body chunk too large: %d > %d", len(chunk), m.maxDataSize)
	}
	return m.write(modifierStateReadWrite, newResponse(wire.Code(wire.ActReplBody), chunk))
}

func (m *modifier) ReplaceBody(r io.Reader) error {
	scanner := milterutil.GetFixedBufferScanner(uint32(m.maxDataSize), r)
	defer scanner.Close()
	for scanner.Scan() {
		err := m.ReplaceBodyRawChunk(scanner.Bytes())
		if err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (m *modifier) Quarantine(reason string) error {
	if m.actions&OptQuarantine == 0 {
		return ErrModificationNotAllowed
	}
	return m.write(modifierStateReadWrite, newResponse(wire.Code(wire.ActQuarantine), []byte(milterutil.NewlineToSpace(reason)+"\x00")))
}

func (m *modifier) AddHeader(name, value string) error {
	if m.actions&OptAddHeader == 0 {
		return ErrModificationNotAllowed
	}
	if !validName(name) {
		return fmt.Errorf("milter: invalid header name: %q", name)
	}
	var buffer bytes.Buffer
	buffer.WriteString(name)
	buffer.WriteByte(0)
	buffer.WriteString(milterutil.CrLfToLf(value))
	buffer.WriteByte(0)
	return m.write(modifierStateReadWrite, newResponse(wire.Code(wire.ActAddHeader), buffer.Bytes()))
}

func (m *modifier) ChangeHeader(index int, name, value string) error {
	if m.actions&OptChangeHeader == 0 {
		return ErrModificationNotAllowed
	}
	if index < 0 || index > math.MaxUint32 {
		return fmt.Errorf("milter: invalid header index: %d", index)
	}
	if !validName(name) {
		return fmt.Errorf("milter: invalid header name: %q", name)
	}
	var buffer bytes.Buffer
	// BigEndian binary representation of the index
	buffer.Write([]byte{byte(index >> 24), byte(index >> 16), byte(index >> 8), byte(index)})
	buffer.WriteString(name)
	buffer.WriteByte(0)
	buffer.WriteString(milterutil.CrLfToLf(value))
	buffer.WriteByte(0)
	return m.write(modifierStateReadWrite, newResponse(wire.Code(wire.ActChangeHeader), buffer.Bytes()))
}

func (m *modifier) InsertHeader(index int, name, value string) error {
	// Insert header does not have its own action flag
	if m.actions&OptChangeHeader == 0 && m.actions&OptAddHeader == 0 {
		return ErrModificationNotAllowed
	}
	if index < 0 || index > math.MaxUint32 {
		return fmt.Errorf("milter: invalid header index: %d", index)
	}
	if !validName(name) {
		return fmt.Errorf("milter: invalid header name: %q", name)
	}
	var buffer bytes.Buffer
	// BigEndian binary representation of the index
	buffer.Write([]byte{byte(index >> 24), byte(index >> 16), byte(index >> 8), byte(index)})
	buffer.WriteString(name)
	buffer.WriteByte(0)
	buffer.WriteString(milterutil.CrLfToLf(value))
	buffer.WriteByte(0)
	return m.write(modifierStateReadWrite, newResponse(wire.Code(wire.ActInsertHeader), buffer.Bytes()))
}

func (m *modifier) ChangeFrom(value string, esmtpArgs string) error {
	if m.version < 6 {
		return ErrVersionTooLow
	}
	if m.actions&OptChangeFrom == 0 {
		return ErrModificationNotAllowed
	}
	var buffer bytes.Buffer
	buffer.WriteString(AddAngle(milterutil.NewlineToSpace(value)))
	buffer.WriteByte(0)
	if esmtpArgs != "" {
		buffer.WriteString(milterutil.NewlineToSpace(esmtpArgs))
		buffer.WriteByte(0)
	}
	return m.write(modifierStateReadWrite, newResponse(wire.Code(wire.ActChangeFrom), buffer.Bytes()))
}

func (m *modifier) Progress() error {
	if m.version < 6 {
		return ErrVersionTooLow
	}
	return m.write(modifierStateReadOnly, respProgress)
}

func (m *modifier) Version() uint32 {
	return m.version
}

func (m *modifier) Protocol() OptProtocol {
	return m.protocol
}

func (m *modifier) Actions() OptAction {
	return m.actions
}

func (m *modifier) MaxDataSize() DataSize {
	return m.maxDataSize
}

func (m *modifier) MilterId() uint64 {
	return m.milterId
}

func (m *modifier) write(requiredState modifierState, resp *Response) error {
	if m.state < requiredState {
		return fmt.Errorf("milter: tried to send action %q in state %d", resp, m.state)
	}
	msg := resp.Response()
	if len(msg.Data) > int(DataSize64K) {
		return fmt.Errorf("milter: invalid data length: %d > %d", len(msg.Data), DataSize64K)
	}
	err := m.writePacket(resp.Response())
	return err
}

func (m *modifier) withState(state modifierState) *modifier {
	if m.state == state {
		return m
	}
	cpy := *m
	cpy.state = state
	return &cpy
}

var _ Modifier = (*modifier)(nil)

// newModifier creates a new [Modifier] instance from s.
func newModifier(s *serverSession, state modifierState) *modifier {
	return &modifier{
		macros:      &macroReader{macrosStages: s.macros},
		state:       state,
		writePacket: s.writePacket,
		version:     s.version,
		protocol:    s.protocol,
		actions:     s.actions,
		maxDataSize: s.maxDataSize,
		milterId:    s.backendId,
	}
}
