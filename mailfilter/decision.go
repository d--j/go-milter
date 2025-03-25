package mailfilter

import (
	"fmt"
	"github.com/d--j/go-milter/milterutil"
	"strconv"
)

type Decision interface {
	getCode() uint16
	getReason() string
}

type decision string

func (d decision) getCode() uint16 {
	c, _ := strconv.ParseUint(string(d[:3]), 10, 16)
	return uint16(c)
}

func (d decision) getReason() string {
	return string(d[4:])
}

const (
	Accept   decision = "250 accept"
	Reject   decision = "550 5.7.1 Command rejected"
	TempFail decision = "451 4.7.1 Service unavailable - try again later"
	Discard  decision = "250 discard"
)

type customResponse struct {
	code   uint16
	reason string
}

func (c customResponse) getCode() uint16 {
	return c.code
}

func (c customResponse) getReason() string {
	return c.reason
}

// CustomErrorResponse can get used to send a custom error response to the client.
// The code must be between 400 and 599.
// The reason can contain new-lines and can start with a valid RFC 2034 extended error code.
// Line ending canonicalization and wrapping is done automatically.
// SMTP line continuation rules (including RFC 2034 extension) are applied automatically. E.g.:
//
//	CustomErrorResponse(550, "5.7.1 Command rejected\nContact support")
//
// will result in:
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

func (c quarantineResponse) getCode() uint16 {
	return 250
}

func (c quarantineResponse) getReason() string {
	if c.reason == "" {
		return "accept (quarantined)"
	}
	if eecEnd := milterutil.FindEnhancedErrorCodeEnd([]byte(c.reason), 250); eecEnd != -1 {
		return c.reason
	}
	return fmt.Sprintf("accept (quarantined: %q)", c.reason)
}

// QuarantineResponse can get used to quarantine a message.
// The reason can contain new-lines and can start with a valid RFC 2034 extended error code.
// The extended error code must have a class of "2".
// See CustomErrorResponse for more information.
func QuarantineResponse(reason string) Decision {
	return &quarantineResponse{
		reason: reason,
	}
}
