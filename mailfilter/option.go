package mailfilter

type options struct {
	decisionAt    DecisionAt
	errorHandling ErrorHandling
	body          *bodyOption
	header        *headerOption
}

type Option func(opt *options)

// DecisionAt defines when the filter decision is made.
type DecisionAt int

const (
	// The DecisionAtConnect constant makes the mail filter call the decision function after the connect event.
	DecisionAtConnect DecisionAt = iota

	// The DecisionAtHelo constant makes the mail filter call the decision function after the HELO/EHLO event.
	DecisionAtHelo

	// The DecisionAtMailFrom constant makes the mail filter call the decision function after the MAIL FROM event.
	DecisionAtMailFrom

	// The DecisionAtData constant makes the mail filter call the decision function after the DATA event (all RCPT TO were sent).
	DecisionAtData

	// The DecisionAtEndOfHeaders constant makes the mail filter call the decision function after the EOH event (all headers were sent).
	DecisionAtEndOfHeaders

	// The DecisionAtEndOfMessage constant makes the mail filter call the decision function at the end of the SMTP transaction.
	// This is the default.
	DecisionAtEndOfMessage
)

// WithDecisionAt sets the decision point for the [MailFilter].
// The default is [DecisionAtEndOfMessage].
func WithDecisionAt(decisionAt DecisionAt) Option {
	return func(opt *options) {
		opt.decisionAt = decisionAt
	}
}

type ErrorHandling int

const (
	// Error just throws the error. The connection to the MTA will break and the MTA will decide what happens to the SMTP transaction.
	Error ErrorHandling = iota
	// AcceptWhenError accepts the transaction despite the error (it gets logged).
	AcceptWhenError
	// TempFailWhenError temporarily rejects the transaction (and logs the error).
	TempFailWhenError
	// RejectWhenError rejects the transaction (and logs the error).
	RejectWhenError
)

// WithErrorHandling sets the error handling for the [MailFilter].
// The default is [TempFailWhenError].
func WithErrorHandling(errorHandling ErrorHandling) Option {
	return func(opt *options) {
		opt.errorHandling = errorHandling
	}
}

type MaxAction int

const (
	// RejectMessageWhenTooBig rejects the message with "552 5.3.4 Maximum allowed body size of %d bytes exceeded." or
	// "552 5.3.4 Maximum allowed header lines (%d) exceeded."
	RejectMessageWhenTooBig MaxAction = iota
	// ClearWhenTooBig will allow the message to pass, but Trx.Body or Trx.Headers will be empty.
	ClearWhenTooBig
	// TruncateWhenTooBig will allow the message to pass,
	// but Trx.Body will be truncated to only the first maxSize bytes
	// or Trx.Headers will be truncated to only the first maxHeaders headers.
	TruncateWhenTooBig
)

type headerOption struct {
	Max       uint32
	MaxAction MaxAction
}

// WithHeader sets the maximum number of headers the [MailFilter] will collect.
// If the number of headers is bigger than maxHeaders, the milter will use maxAction to determine what happens.
//
// If you do not call this function, the default values are:
//   - maxHeaders: 512
//   - maxAction: TruncateWhenTooBig
func WithHeader(maxHeaders uint32, maxAction MaxAction) Option {
	return func(opt *options) {
		opt.header = &headerOption{Max: maxHeaders, MaxAction: maxAction}
	}
}

type bodyOption struct {
	Skip      bool
	MaxMem    int
	MaxSize   int64
	MaxAction MaxAction
}

// WithoutBody configures the [MailFilter] to not request and collect the mail body.
// Use this option when you do not need Trx.Body or Trx.Data in your decision function.
func WithoutBody() Option {
	return func(opt *options) {
		opt.body = &bodyOption{Skip: true}
	}
}

// WithBody configures the [MailFilter] body collection.
// When the message body is bigger than maxMem bytes, the body will be written to a temporary file.
// Otherwise, it will be kept in memory.
// If maxSize is bigger than 0, the milter will stop collecting the body after maxSize bytes
// and use maxAction to determine what happens.
//
// If you do not call this function, the default values are:
//   - maxMem: 200 KiB
//   - maxSize: 100 MiB
//   - maxAction: TruncateWhenTooBig
func WithBody(maxMem int, maxSize int64, maxAction MaxAction) Option {
	return func(opt *options) {
		opt.body = &bodyOption{MaxMem: maxMem, MaxSize: maxSize, MaxAction: maxAction}
	}
}
