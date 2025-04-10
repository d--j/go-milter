package milter

import (
	"fmt"
	"strings"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/d--j/go-milter/milterutil"
)

// Response represents a response structure returned by callback
// handlers to indicate how the milter server should proceed
type Response struct {
	code wire.Code
	data []byte
}

// Response returns message instance with data
func (r *Response) Response() *wire.Message {
	return &wire.Message{Code: r.code, Data: r.data}
}

// Continue returns false if the MTA should stop sending events for this transaction, true otherwise.
// If the Response is for a RCPT TO event, this function will return true if the MTA should accept this recipient.
// A [RespDiscard] Response will return false because the MTA should end sending events for the current
// SMTP transaction to this milter.
func (r *Response) Continue() bool {
	switch wire.ActionCode(r.code) {
	case wire.ActAccept, wire.ActDiscard, wire.ActReject, wire.ActTempFail, wire.ActReplyCode:
		return false
	default:
		return true
	}
}

// String returns a string representation of this response.
// Can be used for logging purposes.
// This method will always return a logfmt compatible string.
// We try to not alter the output of this method arbitrarily – but we do not make any guaranties.
//
// It sometimes internally examines the bytes that will be sent over the wire with the parsing code
// of the client part of this library. This is not the most performant implementation, so
// you might opt to not use this method when your code needs to be performant.
func (r *Response) String() string {
	switch wire.ActionCode(r.code) {
	case wire.ActContinue:
		return "response=continue"
	case wire.ActAccept:
		return "response=accept"
	case wire.ActDiscard:
		return "response=discard"
	case wire.ActReject:
		return "response=reject"
	case wire.ActTempFail:
		return "response=temp_fail"
	case wire.ActSkip:
		return "response=skip"
	case wire.ActProgress:
		return "response=progress"
	case wire.ActReplyCode:
		act, err := parseAction(r.Response())
		if err != nil {
			return fmt.Sprintf("response=invalid code=%d data_len=%d data=%q", r.code, len(r.data), r.data)
		}
		action := "temp_fail"
		if act.SMTPCode > 499 {
			action = "reject"
		}
		return fmt.Sprintf("response=reply_code action=%s code=%d reason=%q", action, act.SMTPCode, act.SMTPReply)
	}
	// Users of the library do not really see modification Response objects.
	// This is just for completeness’ sake
	act, err := parseModifyAct(r.Response())
	if err == nil {
		switch act.Type {
		case ActionAddRcpt:
			if act.RcptArgs != "" {
				return fmt.Sprintf("response=add_rcpt rcpt=%q args=%q", act.Rcpt, act.RcptArgs)
			}
			return fmt.Sprintf("response=add_rcpt rcpt=%q", act.Rcpt)
		case ActionDelRcpt:
			return fmt.Sprintf("response=del_rcpt rcpt=%q", act.Rcpt)
		case ActionQuarantine:
			return fmt.Sprintf("response=quarantine reason=%q", act.Reason)
		case ActionReplaceBody:
			return fmt.Sprintf("response=replace_body len=%d", len(act.Body))
		case ActionChangeFrom:
			if act.FromArgs != "" {
				return fmt.Sprintf("response=change_from from=%q args=%q", act.From, act.FromArgs)
			}
			return fmt.Sprintf("response=change_from from=%q", act.From)
		case ActionAddHeader:
			return fmt.Sprintf("response=add_header name=%q value=%q", act.HeaderName, act.HeaderValue)
		case ActionChangeHeader:
			return fmt.Sprintf("response=change_header name=%q value=%q index=%d", act.HeaderName, act.HeaderValue, act.HeaderIndex)
		case ActionInsertHeader:
			return fmt.Sprintf("response=insert_header name=%q value=%q index=%d", act.HeaderName, act.HeaderValue, act.HeaderIndex)
		}
	}
	return fmt.Sprintf("response=unknown code=%d data_len=%d data=%q", r.code, len(r.data), r.data)
}

// newResponse generates a new Response suitable for [wire.WritePacket]
func newResponse(code wire.Code, data []byte) *Response {
	return &Response{code, data}
}

// newResponseStr generates a new [Response] with string payload (null-byte terminated)
func newResponseStr(code wire.Code, data string) (*Response, error) {
	if len(data) > int(DataSize64K)-1 { // space for null-bytes
		return nil, fmt.Errorf("milter: invalid data length: %d > %d", len(data), int(DataSize64K)-1)
	}
	if strings.ContainsRune(data, 0) {
		return nil, fmt.Errorf("milter: invalid data: cannot contain null-bytes")
	}
	return newResponse(code, []byte(data+"\x00")), nil
}

// RejectWithCodeAndReason stops processing and tells client the error code and reason to sent
//
// smtpCode must be between 400 and 599, otherwise this method will return an error.
// See [milterutil.FormatResponse] for the rules on the reason string.
func RejectWithCodeAndReason(smtpCode uint16, reason string) (*Response, error) {
	if smtpCode < 400 || smtpCode > 599 {
		return nil, fmt.Errorf("milter: invalid code %d", smtpCode)
	}
	data, err := milterutil.FormatResponse(smtpCode, reason)
	if err != nil {
		return nil, err
	}
	return newResponseStr(wire.Code(wire.ActReplyCode), data)
}

// Define standard responses with no data
var (
	// RespAccept signals to the MTA that the current transaction should be accepted.
	// No more events get send to the milter after this response.
	RespAccept = &Response{code: wire.Code(wire.ActAccept)}

	// RespContinue signals to the MTA that the current transaction should continue
	RespContinue = &Response{code: wire.Code(wire.ActContinue)}

	// RespDiscard signals to the MTA that the current transaction should be silently discarded.
	// No more events get send to the milter after this response.
	RespDiscard = &Response{code: wire.Code(wire.ActDiscard)}

	// RespReject signals to the MTA that the current transaction should be rejected with a hard rejection.
	// No more events get send to the milter after this response.
	RespReject = &Response{code: wire.Code(wire.ActReject)}

	// RespTempFail signals to the MTA that the current transaction should be rejected with a temporary error code.
	// The sending MTA might try to deliver the same message again at a later time.
	// No more events get send to the milter after this response.
	RespTempFail = &Response{code: wire.Code(wire.ActTempFail)}

	// RespSkip signals to the MTA that transaction should continue and that the MTA
	// does not need to send more events of the same type. This response one makes sense/is possible as
	// return value of [Milter.RcptTo], [Milter.Header] and [Milter.BodyChunk].
	// No more events get send to the milter after this response.
	RespSkip = &Response{code: wire.Code(wire.ActSkip)}
)

// respProgress signals to the MTA that the milter does progress and prevents the MTA to quit the connection
var respProgress = &Response{code: wire.Code(wire.ActProgress)}
