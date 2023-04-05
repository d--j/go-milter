package mailfilter

import (
	"io"

	"github.com/d--j/go-milter/mailfilter/addr"
	"github.com/d--j/go-milter/mailfilter/header"
)

// Trx can be used to examine the data of the current mail transaction and
// also send changes to the message back to the MTA.
type Trx interface {
	// MTA hols information about the connecting MTA
	MTA() *MTA
	// Connect holds the [Connect] information of this transaction.
	Connect() *Connect
	// Helo holds the [Helo] information of this transaction.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtConnect].
	Helo() *Helo

	// MailFrom holds the [MailFrom] of this transaction.
	// You can change this and your changes get send back to the MTA.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtHelo].
	MailFrom() *addr.MailFrom
	ChangeMailFrom(from string, esmtpArgs string)

	// RcptTos holds the [RcptTo] recipient slice of this transaction.
	// You can change this and your changes get send back to the MTA.
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
	AddRcptTo(rcptTo string, esmtpArgs string)
	// DelRcptTo deletes the rcptTo (without angles) from the list of recipients.
	//
	// rcptTo gets compared to the existing recipients IDNA address aware.
	DelRcptTo(rcptTo string)

	// Headers are the [Header] fields of this message.
	// You can use methods of this to change the header fields of the current message.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtData].
	Headers() header.Header

	// Body gets you a [io.ReadSeeker] of the body. The reader seeked to the start of the body.
	//
	// This method returns nil when you used [WithDecisionAt] with anything other than [DecisionAtEndOfMessage]
	// or you used [WithoutBody].
	Body() io.ReadSeeker
	// ReplaceBody replaces the body of the current message with the contents
	// of the [io.Reader] r.
	ReplaceBody(r io.Reader)

	// QueueId is the queue ID the MTA assigned for this transaction.
	// You cannot change this value.
	//
	// Only populated if [WithDecisionAt] is bigger than [DecisionAtMailFrom].
	QueueId() string
}
