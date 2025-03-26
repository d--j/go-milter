package milterutil

import (
	"fmt"
	"golang.org/x/text/transform"
	"strings"
)

// MaxResponseSize is the maximum size of a response string in bytes.
// It is 64 KiB - 2 bytes.
// It is unknown if all MTA implementations can handle such long responses.
// But it is the technical maximum size of a response string (One milter packet is 64KB minus 1 byte for the null-byte and 1 byte for the command byte).
const MaxResponseSize = 64*1024*1024 - 2

// FormatResponse generates an SMTP response string.
// smtpCode must be between 100 and 599, otherwise this function returns an error.
// reason is the human-readable reason for the response (UTF-8 encoded). It can start with an RFC 2034 Enhanced Error Code.
// The response is formatted as a multi-line response when (a) the reason already contains new-lines, or (b) the lines would get longer than 950 bytes.
// "\n" line endings in reason get canonicalized to "\r\n". "%" in reason get replaced with "%%".
// This function returns an error when the resulting SMTP text has a length of more than [DataSize64K] - 1 (65534 bytes).
//
// Some examples:
//
//	FormatResponse(250, "Accept") // "250 Accept"
//	FormatResponse(250, "%") // "250 %%"
//	FormatResponse(550, "5.7.1 Command rejected") // "550 5.7.1 Command rejected"
//	FormatResponse(550, "5.7.1 Command rejected\nContact support") // "550-5.7.1 Command rejected\r\n550 5.7.1 Contact support"
//
// See https://www.iana.org/assignments/smtp-enhanced-status-codes/smtp-enhanced-status-codes.xhtml for a list of extended error codes and when to use them.
func FormatResponse(smtpCode uint16, reason string) (string, error) {
	if smtpCode < 100 || smtpCode > 599 {
		return "", fmt.Errorf("milter: invalid code %d", smtpCode)
	}
	// bail early if the reason is way too long
	if len(reason) > MaxResponseSize-4 {
		return "", fmt.Errorf("milter: reason too long: %d > %d", len(reason), MaxResponseSize-4)
	}
	escapeAndNormalize := transform.Chain(&DoublePercentTransformer{}, &CrLfCanonicalizationTransformer{})
	data, _, _ := transform.String(escapeAndNormalize, strings.TrimRight(reason, "\r\n"))
	data, _, _ = transform.String(&MaximumLineLengthTransformer{}, data)
	data, _, _ = transform.String(&SMTPReplyTransformer{Code: smtpCode}, data)
	if len(data) > MaxResponseSize {
		return "", fmt.Errorf("milter: formatted reason too long: %d > %d", len(data), MaxResponseSize)
	}
	return data, nil
}
