package milter

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/textproto"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/d--j/go-milter/milterutil"
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

type Action struct {
	Type ActionType

	// SMTP code if milter wants to abort the connection/message. Zero otherwise.
	SMTPCode uint16
	// Properly formatted reply text if milter wants to abort the connection/message. Empty string otherwise.
	SMTPReply string
}

// StopProcessing returns true when the milter wants to immediately stop this SMTP connection.
// You can use [Action.SMTPReply] to send as reply to the current SMTP command.
func (a Action) StopProcessing() bool {
	return a.SMTPCode > 0
}

func parseAction(msg *wire.Message) (*Action, error) {
	act := &Action{}

	switch wire.ActionCode(msg.Code) {
	case wire.ActAccept:
		act.Type = ActionAccept
	case wire.ActContinue:
		act.Type = ActionContinue
	case wire.ActDiscard:
		act.Type = ActionDiscard
	case wire.ActReject:
		act.Type = ActionReject
	case wire.ActTempFail:
		act.Type = ActionTempFail
	case wire.ActSkip:
		act.Type = ActionSkip
	case wire.ActReplyCode:
		if len(msg.Data) <= 4 {
			return nil, fmt.Errorf("action read: unexpected data length: %d", len(msg.Data))
		}
		checker := textproto.NewReader(bufio.NewReader(bytes.NewReader(msg.Data)))
		// this also accepts FTP style multi-line responses as valid
		// It's highly unlikely that milter sends one of those, so we ignore this false positive
		code, _, err := checker.ReadResponse(0)
		if err != nil {
			return nil, fmt.Errorf("action read: malformed SMTP response: %q", msg.Data)
		}
		act.Type = ActionRejectWithCode
		act.SMTPCode = uint16(code)
		act.SMTPReply = wire.ReadCString(msg.Data) // use raw response as it was formatted by milter
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
	// E.g. HeaderIndex = 3 and HdrName = "DKIM-Signature" mean "change third field with the canonical header name Dkim-Signature".
	// Order is the same as of HeaderField calls.
	//
	// If Type = ActionInsertHeader the index is global to all headers, 1-based and means "insert after the HeaderIndex header".
	// A HeaderIndex of 0 has the special meaning "at the very beginning".
	//
	// Deleted headers (Type = ActionChangeHeader and HeaderValue == "") may change the indexes of the other headers.
	// Postfix MTA removes the header from the linked list (and thus change the indexes of headers coming after the deleted header).
	// Sendmail on the other hand will only mark the header as deleted.
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

func parseModifyAct(msg *wire.Message) (*ModifyAction, error) {
	act := &ModifyAction{}

	switch wire.ModifyActCode(msg.Code) {
	case wire.ActAddRcpt:
		argv := bytes.Split(msg.Data, []byte{0x00})
		if len(argv) != 2 {
			return nil, fmt.Errorf("read modify action: wrong number of arguments %d for ActAddRcpt action", len(argv))
		}
		act.Type = ActionAddRcpt
		act.Rcpt = string(argv[0])
	case wire.ActAddRcptPar:
		argv := bytes.Split(msg.Data, []byte{0x00})
		if len(argv) > 3 || len(argv) < 2 {
			return nil, fmt.Errorf("read modify action: wrong number of arguments %d for ActAddRcpt action", len(argv))
		}
		act.Type = ActionAddRcpt
		act.Rcpt = string(argv[0])
		if len(argv) == 3 {
			act.RcptArgs = string(argv[1])
		}
	case wire.ActDelRcpt:
		act.Type = ActionDelRcpt
		act.Rcpt = wire.ReadCString(msg.Data)
	case wire.ActQuarantine:
		act.Type = ActionQuarantine
		act.Reason = wire.ReadCString(msg.Data)
	case wire.ActReplBody:
		act.Type = ActionReplaceBody
		act.Body = msg.Data
	case wire.ActChangeFrom:
		argv := bytes.Split(msg.Data, []byte{0x00})
		if len(argv) > 3 || len(argv) < 2 {
			return nil, fmt.Errorf("read modify action: wrong number of arguments %d for ActChangeFrom action", len(argv))
		}
		act.Type = ActionChangeFrom
		act.From = string(argv[0])
		if len(argv) == 3 {
			act.FromArgs = string(argv[1])
		}
	case wire.ActChangeHeader, wire.ActInsertHeader:
		if len(msg.Data) < 4 {
			return nil, fmt.Errorf("read modify action: missing header index")
		}
		if wire.ModifyActCode(msg.Code) == wire.ActChangeHeader {
			act.Type = ActionChangeHeader
		} else {
			act.Type = ActionInsertHeader
		}
		act.HeaderIndex = binary.BigEndian.Uint32(msg.Data)

		// Sendmail 8 compatibility
		if wire.ModifyActCode(msg.Code) == wire.ActChangeHeader && act.HeaderIndex == 0 {
			act.HeaderIndex = 1
		}

		msg.Data = msg.Data[4:]
		fallthrough
	case wire.ActAddHeader:
		argv := bytes.Split(msg.Data, []byte{0x00})
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

// Modifier provides access to [Macros] to callback handlers. It also defines a
// number of functions that can be used by callback handlers to modify processing of the email message.
// Besides [Modifier.Progress] they can only be called in the EndOfMessage callback.
type Modifier struct {
	Macros              Macros
	writeProgressPacket func(*wire.Message) error
	writePacket         func(*wire.Message) error
	actions             OptAction
	maxDataSize         DataSize
}

func hasHats(str string) bool {
	return len(str) > 1 && str[0] == '<' && str[len(str)-1] == '>'
}

func addHats(str string) string {
	if hasHats(str) {
		return str
	} else {
		return fmt.Sprintf("<%s>", str)
	}
}

func removeHats(str string) string {
	if hasHats(str) {
		return str[1 : len(str)-1]
	} else {
		return str
	}
}

var ErrModificationNotAllowed = errors.New("milter: modification not allowed via milter protocol negotiation")

// AddRecipient appends a new envelope recipient for current message.
// You can optionally specify esmtpArgs to pass along. You need to negotiate this via [OptAddRcptWithArgs] with the MTA.
func (m *Modifier) AddRecipient(r string, esmtpArgs string) error {
	if m.actions&OptAddRcpt == 0 && m.actions&OptAddRcptWithArgs == 0 {
		return ErrModificationNotAllowed
	}
	if esmtpArgs != "" && m.actions&OptAddRcptWithArgs == 0 {
		return ErrModificationNotAllowed
	}
	code := wire.ActAddRcpt
	var buffer bytes.Buffer
	buffer.WriteString(addHats(r))
	buffer.WriteByte(0)
	// send wire.ActAddRcptPar when that is the only allowed action, or we need to send it because esmptArgs ist set
	if (esmtpArgs != "" && m.actions&OptAddRcptWithArgs != 0) || (esmtpArgs == "" && m.actions&OptAddRcpt == 0) {
		buffer.WriteString(esmtpArgs)
		buffer.WriteByte(0)
		code = wire.ActAddRcptPar
	}
	return m.writePacket(newResponse(wire.Code(code), buffer.Bytes()).Response())
}

// DeleteRecipient removes an envelope recipient address from message
func (m *Modifier) DeleteRecipient(r string) error {
	if m.actions&OptRemoveRcpt == 0 {
		return ErrModificationNotAllowed
	}
	resp, err := newResponseStr(wire.Code(wire.ActDelRcpt), addHats(r))
	if err != nil {
		return err
	}
	return m.writePacket(resp.Response())
}

// ReplaceBodyRawChunk sends one chunk of the body replacement.
//
// The chunk get send as-is. Caller needs to ensure that the chunk does not exceed the maximum configured data size (defaults to [DataSize64K])
//
// You should do the ReplaceBodyRawChunk calls all in one go without intersecting it with other modification actions.
// MTAs like Postfix do not allow that.
func (m *Modifier) ReplaceBodyRawChunk(chunk []byte) error {
	if m.actions&OptChangeBody == 0 {
		return ErrModificationNotAllowed
	}
	if len(chunk) > int(m.maxDataSize) {
		return fmt.Errorf("milter: body chunk too large: %d > %d", len(chunk), m.maxDataSize)
	}
	return m.writePacket(newResponse(wire.Code(wire.ActReplBody), chunk).Response())
}

// ReplaceBody reads from r and send its contents in the least amount of chunks to the MTA.
//
// This function does not do any CR LF line ending canonicalization or maximum line length enforcements.
// If you need that you can use the various transform.Transformers of this package to wrap your reader.
//
//	t := transform.Chain(&milter.CrLfCanonicalizationTransformer{}, &milter.MaximumLineLengthTransformer{})
//	wrappedR := transform.NewReader(r, t)
//	m.ReplaceBody(wrappedR)
//
// This function tries to use as few calls to [Modifier.ReplaceBodyRawChunk] as possible.
//
// You can call ReplaceBody multiple times. The MTA will combine all those calls into one message.
//
// You should do the ReplaceBody calls all in one go without intersecting it with other modification actions.
// MTAs like Postfix do not allow that.
func (m *Modifier) ReplaceBody(r io.Reader) error {
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

// Quarantine a message by giving a reason to hold it
func (m *Modifier) Quarantine(reason string) error {
	if m.actions&OptQuarantine == 0 {
		return ErrModificationNotAllowed
	}
	return m.writePacket(newResponse(wire.Code(wire.ActQuarantine), []byte(reason+"\x00")).Response())
}

// AddHeader appends a new email message header to the message
//
// Unfortunately when interacting with Sendmail it is not guaranteed that the header
// will be added at the end. If Sendmail has a (maybe deleted) header of the same name
// in the list of headers, this header will be altered/re-used instead of adding a new
// header at the end.
//
// If you always want to add the header at the very end you need to use InsertHeader with
// a very high index.
func (m *Modifier) AddHeader(name, value string) error {
	if m.actions&OptAddHeader == 0 {
		return ErrModificationNotAllowed
	}
	var buffer bytes.Buffer
	buffer.WriteString(name)
	buffer.WriteByte(0)
	buffer.WriteString(milterutil.CrLfToLf(value))
	buffer.WriteByte(0)
	return m.writePacket(newResponse(wire.Code(wire.ActAddHeader), buffer.Bytes()).Response())
}

// ChangeHeader replaces the header at the specified position with a new one.
// The index is per canonical name and one-based. To delete a header pass an empty value.
// If the index is bigger than there are headers with that name, then ChangeHeader will actually
// add a new header at the end of the header list (With the same semantic as AddHeader).
func (m *Modifier) ChangeHeader(index int, name, value string) error {
	if m.actions&OptChangeHeader == 0 {
		return ErrModificationNotAllowed
	}
	var buffer bytes.Buffer
	if err := binary.Write(&buffer, binary.BigEndian, uint32(index)); err != nil {
		return err
	}
	buffer.WriteString(name)
	buffer.WriteByte(0)
	buffer.WriteString(milterutil.CrLfToLf(value))
	buffer.WriteByte(0)
	return m.writePacket(newResponse(wire.Code(wire.ActChangeHeader), buffer.Bytes()).Response())
}

// InsertHeader inserts the header at the specified position.
// index is one-based. The index 0 means at the very beginning.
//
// Unfortunately when interacting with Sendmail the index is used to find the position
// in Sendmail's internal list of headers. Not all of those internal headers get send to the milter.
// Thus, you cannot really add a header at a specific position when the milter client is Sendmail.
func (m *Modifier) InsertHeader(index int, name, value string) error {
	// Insert header does not have its own action flag
	if m.actions&OptChangeHeader == 0 && m.actions&OptAddHeader == 0 {
		return ErrModificationNotAllowed
	}
	var buffer bytes.Buffer
	if err := binary.Write(&buffer, binary.BigEndian, uint32(index)); err != nil {
		return err
	}
	buffer.WriteString(name)
	buffer.WriteByte(0)
	buffer.WriteString(milterutil.CrLfToLf(value))
	buffer.WriteByte(0)
	return m.writePacket(newResponse(wire.Code(wire.ActInsertHeader), buffer.Bytes()).Response())
}

// ChangeFrom replaces the FROM envelope header with a new one
func (m *Modifier) ChangeFrom(value string, esmtpArgs string) error {
	if m.actions&OptChangeFrom == 0 {
		return ErrModificationNotAllowed
	}
	var buffer bytes.Buffer
	buffer.WriteString(addHats(value))
	buffer.WriteByte(0)
	if esmtpArgs != "" {
		buffer.WriteString(esmtpArgs)
		buffer.WriteByte(0)
	}
	return m.writePacket(newResponse(wire.Code(wire.ActChangeFrom), buffer.Bytes()).Response())
}

var respProgress = &Response{code: wire.Code(wire.ActProgress)}

// Progress tells the client that there is progress in a long operation
func (m *Modifier) Progress() error {
	return m.writeProgressPacket(respProgress.Response())
}

func errorWriteReadOnly(m *wire.Message) error {
	return fmt.Errorf("tried to send action %c in read-only state", m.Code)
}

// newModifier creates a new [Modifier] instance from s. If it is readOnly then all modification actions will throw an error.
func newModifier(s *serverSession, readOnly bool) *Modifier {
	writePacket := s.writePacket
	if readOnly {
		writePacket = errorWriteReadOnly
	}
	return &Modifier{
		Macros:              &macroReader{macrosStages: s.macros},
		writePacket:         writePacket,
		writeProgressPacket: s.writePacket,
		actions:             s.actions,
		maxDataSize:         s.maxDataSize,
	}
}

// NewTestModifier is only exported for unit-tests. It can only be use internally since it uses the internal package [wire].
func NewTestModifier(macros Macros, writePacket, writeProgress func(msg *wire.Message) error, actions OptAction, maxDataSize DataSize) *Modifier {
	return &Modifier{
		Macros:              macros,
		writePacket:         writePacket,
		writeProgressPacket: writeProgress,
		actions:             actions,
		maxDataSize:         maxDataSize,
	}
}
