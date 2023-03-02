package mailfilter

import "strconv"

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

func CustomErrorResponse(code uint16, reason string) Decision {
	return &customResponse{
		code:   code,
		reason: reason,
	}
}
