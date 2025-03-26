package mailfilter

import (
	"github.com/d--j/go-milter/milterutil"
)

type Decision interface {
	// String returns the decision as an SMTP response string.
	// This is useful for testing/logging.
	String() string
	// Equal returns true if the decision is semantically equal to the provided decision.
	// This is useful for testing.
	// As a special case, a QuarantineResponse is equal to Accept.
	Equal(Decision) bool
}

// decision is a string backed Decision implementation that gets translated to the default milter responses
// milter.RespAccept, milter.RespReject, milter.RespTempFail, milter.RespDiscard.
type decision string

func (d decision) String() string {
	return string(d)
}

func (d decision) Equal(d2 Decision) bool {
	return d2 != nil && d.String() == d2.String()
}

var _ Decision = (*decision)(nil)

const (
	// Accept is a decision that tells the MTA to accept the message (milter.RespAccept).
	Accept decision = "250 accept"
	// Reject is a decision that tells the MTA to reject the message (milter.RespReject).
	Reject decision = "550 5.7.1 Command rejected"
	// TempFail is a decision that tells the MTA to temporarily fail the message (milter.RespTempFail).
	TempFail decision = "451 4.7.1 Service unavailable - try again later"
	// Discard is a decision that tells the MTA to discard the message (milter.RespDiscard).
	// The SMTP client does not get notified about this decision and must assume that the SMTP message was successfully delivered.
	Discard decision = "250 discard"
)

type customResponse struct {
	code   uint16
	reason string
}

func (c customResponse) String() string {
	formatted, err := milterutil.FormatResponse(c.code, c.reason)
	if err != nil {
		panic(err)
	}
	return formatted
}

func (c customResponse) Equal(d Decision) bool {
	return d != nil && c.String() == d.String()
}

// CustomErrorResponse can get used to send a custom error response to the client.
// The code must be between 400 and 599.
// The reason can contain new-lines and can start with a valid RFC 2034 extended error code.
// Line ending canonicalization and wrapping is done automatically.
// SMTP line continuation rules (including RFC 2034 extension) are applied automatically. E.g.:
//
//	CustomErrorResponse(550, "5.7.1 Command rejected\nContact support")
//
// will result in this SMTP response:
//
//	550-5.7.1 Command rejected\r\n
//	550 5.7.1 Contact support
func CustomErrorResponse(code uint16, reason string) Decision {
	return &customResponse{
		code:   code,
		reason: reason,
	}
}

type quarantineResponse struct {
	reason string
}

func (c quarantineResponse) String() string {
	return Accept.String()
}

func (c quarantineResponse) Equal(d Decision) bool {
	return d != nil && c.String() == d.String()
}

// QuarantineResponse can get used to quarantine a message.
// The message will be accepted but quarantined.
// You cannot provide extended error codes or multiline responses, since reason will be used as the quarantine reason and
// will not be passed to the client.
func QuarantineResponse(reason string) Decision {
	return &quarantineResponse{
		reason: reason,
	}
}
