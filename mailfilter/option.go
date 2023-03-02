package mailfilter

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

type ErrorHandling int

const (
	// Error just throws the error
	Error ErrorHandling = iota
	// AcceptWhenError accepts the transaction despite the error (it gets logged).
	AcceptWhenError
	// TempFailWhenError temporarily rejects the transaction (and logs the error).
	TempFailWhenError
	// RejectWhenError rejects the transaction (and logs the error).
	RejectWhenError
)

type options struct {
	decisionAt    DecisionAt
	errorHandling ErrorHandling
	skipBody      bool
	syslogPrefix  string
}

type Option func(opt *options)

// WithDecisionAt sets the decision point for the [MailFilter].
// The default is [DecisionAtEndOfMessage].
func WithDecisionAt(decisionAt DecisionAt) Option {
	return func(opt *options) {
		opt.decisionAt = decisionAt
	}
}

// WithErrorHandling sets the error handling for the [MailFilter].
// The default is [TempFailWhenError].
func WithErrorHandling(errorHandling ErrorHandling) Option {
	return func(opt *options) {
		opt.errorHandling = errorHandling
	}
}

// WithoutBody configures the [MailFilter] to not request and collect the mail body.
func WithoutBody() Option {
	return func(opt *options) {
		opt.skipBody = true
	}
}

// WithSyslog enables logging to syslog with a prefix of prefix.
// This is a global option.
// All calls to [github.com/d--j/go-milter.LogWarning] will be also send to the syslog.
func WithSyslog(prefix string) Option {
	return func(opt *options) {
		opt.syslogPrefix = prefix
	}
}
