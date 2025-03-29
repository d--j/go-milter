package mailfilter

import (
	"io"

	"github.com/d--j/go-milter/mailfilter/addr"
	"github.com/d--j/go-milter/mailfilter/header"
)

// Trx can be used to examine the data of the current mail transaction and
// also send changes to the message back to the MTA.
type Trx interface {
	// MTA holds information about the connecting MTA
	MTA() *MTA
	// Connect holds the [Connect] information of this transaction.
	Connect() *Connect
	// Helo holds the [Helo] information of this transaction.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtConnect].
	Helo() *Helo

	// MailFrom holds the [MailFrom] of this transaction.
	// Your changes to this pointer's Addr and Args values get send back to the MTA.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtHelo].
	MailFrom() *addr.MailFrom
	// ChangeMailFrom changes the MailFrom Addr and Args.
	// This is just a convenience method, you could also directly change the MailFrom.
	//
	// When your filter should work with Sendmail you should set esmtpArgs to the empty string
	// since Sendmail validates the provided esmtpArgs and also rejects valid values like `SIZE=20`.
	ChangeMailFrom(from string, esmtpArgs string)

	// RcptTos holds the [RcptTo] recipient slice of this transaction.
	// Your changes to Addr and/or Args values of the elements of this slice get send back to the MTA.
	// But you should use DelRcptTo and AddRcptTo
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtMailFrom].
	RcptTos() []*addr.RcptTo
	// HasRcptTo returns true when rcptTo is in the list of recipients.
	//
	// rcptTo gets compared to the existing recipients IDNA address aware.
	HasRcptTo(rcptTo string) bool
	// AddRcptTo adds the rcptTo (without angles) to the list of recipients with the ESMTP arguments esmtpArgs.
	// If rcptTo is already in the list of recipients only the esmtpArgs of this recipient get updated.
	//
	// rcptTo gets compared to the existing recipients IDNA address aware.
	//
	// When your filter should work with Sendmail you should set esmtpArgs to the empty string
	// since Sendmail validates the provided esmtpArgs and also rejects valid values like `BODY=8BITMIME`.
	AddRcptTo(rcptTo string, esmtpArgs string)
	// DelRcptTo deletes the rcptTo (without angles) from the list of recipients.
	//
	// rcptTo gets compared to the existing recipients IDNA address aware.
	DelRcptTo(rcptTo string)

	// Headers are the [Header] fields of this message.
	// You can use methods of [Header] to change the header fields of the current message.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtData].
	Headers() header.Header
	// HeadersEnforceOrder activates a workaround for Sendmail to ensure that the header ordering of the resulting email
	// is exactly the same as the order in Headers. To ensure that, we delete all existing headers and add all headers
	// as new headers. This is of course a significant overhead, so you should only call this method when you really need
	// to enforce a specific header order.
	//
	// Sendmail may re-fold your header values (newline characters you inserted might get removed).
	//
	// For other MTAs this method does not do anything (since there we can ensure correct header ordering without this workaround).
	HeadersEnforceOrder()

	// Body gets you a [io.ReadSeeker] of the body.
	// The reader gets seeked to the start of the body whenever you call this method.
	//
	// This method returns nil when you used [WithDecisionAt] with anything other than [DecisionAtEndOfMessage]
	// or you used [WithoutBody].
	Body() io.ReadSeeker
	// ReplaceBody replaces the body of the current message with the contents of the [io.Reader] r.
	// The reader will only get read once, but it might get buffered when you call [Data] on the transaction.
	// When the reader implements the [io.Closer] interface, the milter will call [io.Closer.Close] on the reader when it is done with it.
	ReplaceBody(r io.Reader)

	// QueueId is the queue ID the MTA assigned for this transaction.
	// You cannot change this value.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtMailFrom].
	QueueId() string

	// Data returns the full email data (headers and body) of the current message.
	// It includes any modifications you made to the Headers and either uses Body or ReplaceBody as the body of the message.
	// If you set WithoutBody, WithDecisionAt is not DecisionAtEndOfMessage, or the body is bigger than the configured maximum body size, Data will be the same as [header.Header.Reader].
	// A call to Data might re-use the Body io.ReadSeeker. Using Data and Body at the same time is not supported.
	Data() io.Reader
}
