package milter

import (
	"fmt"
	"strings"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/d--j/go-milter/milterutil"
	"golang.org/x/text/transform"
)

// Response represents a response structure returned by callback
// handlers to indicate how the milter server should proceed
type Response struct {
	code wire.Code
	data []byte
}

// Response returns message instance with data
func (c *Response) Response() *wire.Message {
	return &wire.Message{Code: c.code, Data: c.data}
}

// Continue returns false if the MTA should stop sending events for this transaction, true otherwise.
// A RespDiscard Response will return false because the MTA should end sending events for the current
// SMTP transaction to this milter.
func (c *Response) Continue() bool {
	switch wire.ActionCode(c.code) {
	case wire.ActAccept, wire.ActDiscard, wire.ActReject, wire.ActTempFail, wire.ActReplyCode:
		return false
	default:
		return true
	}
}

// newResponse generates a new Response suitable for wire.WritePacket
func newResponse(code wire.Code, data []byte) *Response {
	return &Response{code, data}
}

// newResponseStr generates a new Response with string payload (null-byte terminated)
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
//
// The reason can contain new-lines. Line ending canonicalization is done automatically.
// This function returns an error when the resulting SMTP text has a length of more than [DataSize64K] - 1
func RejectWithCodeAndReason(smtpCode uint16, reason string) (*Response, error) {
	if smtpCode < 400 || smtpCode > 599 {
		return nil, fmt.Errorf("milter: invalid code %d", smtpCode)
	}
	if len(reason) > int(DataSize64K)-5 {
		return nil, fmt.Errorf("milter: reason too long: %d > %d", len(reason), int(DataSize64K)-5)
	}
	escapeAndNormalize := transform.Chain(&milterutil.DoublePercentTransformer{}, &milterutil.CrLfCanonicalizationTransformer{})
	data, _, err := transform.String(escapeAndNormalize, strings.TrimRight(reason, "\r\n"))
	if err != nil {
		return nil, err
	}
	data, _, err = transform.String(&milterutil.MaximumLineLengthTransformer{}, data)
	if err != nil {
		return nil, err
	}
	data, _, err = transform.String(&milterutil.SMTPReplyTransformer{Code: smtpCode}, data)
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
	// return value of Milter.RcptTo, Milter.Header and Milter.BodyChunk.
	// No more events get send to the milter after this response.
	RespSkip = &Response{code: wire.Code(wire.ActSkip)}
)
