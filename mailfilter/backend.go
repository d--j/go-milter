package mailfilter

import (
	"context"
	"errors"
	"fmt"
	"github.com/d--j/go-milter/internal/body"
	"github.com/d--j/go-milter/internal/header"
	"strings"
	"time"

	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/mailfilter/addr"
)

type backend struct {
	milter.NoOpMilter
	opts         options
	leadingSpace bool
	decision     DecisionModificationFunc
	transaction  *transaction
	headerCount  uint64
	bodySize     int64
}

func (b *backend) decideOrContinue(stage DecisionAt, m milter.Modifier) (*milter.Response, error) {
	if b.opts.decisionAt == stage && !b.transaction.hasDecision {
		b.makeDecision(m)
		if !b.transaction.hasModifications() {
			if b.transaction.decisionErr != nil {
				return b.error(b.transaction.decisionErr)
			}
			response := b.transaction.response()
			b.readyForNewMessage()
			return response, nil
		}
	}
	return milter.RespContinue, nil
}

func (b *backend) error(err error) (*milter.Response, error) {
	b.Cleanup(nil)
	switch b.opts.errorHandling {
	case Error:
		return nil, err
	case AcceptWhenError:
		milter.LogWarning("milter: accept message despite error: %s", err)
		return milter.RespAccept, err
	case TempFailWhenError:
		milter.LogWarning("milter: temp fail message because of error: %s", err)
		return milter.RespTempFail, err
	case RejectWhenError:
		milter.LogWarning("milter: reject message because of error: %s", err)
		return milter.RespReject, err
	default:
		panic(b.opts.errorHandling)
	}
}

func (b *backend) makeDecision(m milter.Modifier) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		b.transaction.makeDecision(ctx, b.decision)
		done <- struct{}{}
	}()
	for {
		select {
		case <-done:
			cancel()
			return
		case <-ticker.C:
			err := m.Progress()
			if err != nil && !errors.Is(err, milter.ErrVersionTooLow) {
				// instruct decision function to abort
				cancel()
				// wait for decision function
				<-done
				// if there was no error in the decision function (e.g. it did not actually check ctx.Done())
				// set the Progress error so that we will not actually think we should continue
				if b.transaction.decisionErr == nil {
					b.transaction.decisionErr = err
				}
				return
			}
		}
	}
}

func (b *backend) NewConnection(m milter.Modifier) error {
	b.Cleanup(m)
	return nil
}

func (b *backend) Connect(host string, family string, port uint16, addr string, m milter.Modifier) (*milter.Response, error) {
	b.Cleanup(m)
	b.transaction.mta = MTA{
		Version: m.Get(milter.MacroMTAVersion),
		FQDN:    m.Get(milter.MacroMTAFQDN),
		Daemon:  m.Get(milter.MacroDaemonName),
	}
	b.transaction.connect = Connect{
		Host:   host,
		Family: family,
		Port:   port,
		Addr:   addr,
		IfName: m.Get(milter.MacroIfName),
		IfAddr: m.Get(milter.MacroIfAddr),
	}
	return b.decideOrContinue(DecisionAtConnect, m)
}

func (b *backend) Helo(name string, m milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	b.transaction.helo = Helo{
		Name:        name,
		TlsVersion:  m.Get(milter.MacroTlsVersion),
		Cipher:      m.Get(milter.MacroCipher),
		CipherBits:  m.Get(milter.MacroCipherBits),
		CertSubject: m.Get(milter.MacroCertSubject),
		CertIssuer:  m.Get(milter.MacroCertIssuer),
	}
	return b.decideOrContinue(DecisionAtHelo, m)
}

func (b *backend) MailFrom(from string, esmtpArgs string, m milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	b.transaction.origMailFrom = addr.NewMailFrom(from, esmtpArgs, m.Get(milter.MacroMailMailer), m.Get(milter.MacroAuthAuthen), m.Get(milter.MacroAuthType))
	return b.decideOrContinue(DecisionAtMailFrom, m)
}

func (b *backend) RcptTo(rcptTo string, esmtpArgs string, m milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		if m.Protocol()&milter.OptSkip != 0 {
			return milter.RespSkip, nil
		}
		return milter.RespContinue, nil
	}
	if b.opts.rcptToValidator != nil {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		ctx, cancel := context.WithCancel(context.Background())
		type ret struct {
			Decision Decision
			Err      error
		}
		done := make(chan ret)
		go func() {
			mtaCopy := *b.transaction.MTA()
			connectCopy := *b.transaction.Connect()
			heloCopy := *b.transaction.Helo()
			dec, err := b.opts.rcptToValidator(ctx, &RcptToValidationInput{
				MTA:      &mtaCopy,
				Connect:  &connectCopy,
				Helo:     &heloCopy,
				MailFrom: b.transaction.MailFrom().Copy(),
				RcptTo:   addr.NewRcptTo(rcptTo, esmtpArgs, m.Get(milter.MacroRcptMailer)),
			})
			done <- ret{dec, err}
		}()
		for {
			select {
			case r := <-done:
				cancel()
				if r.Err != nil {
					return b.error(r.Err)
				}
				if r.Decision == nil || r.Decision.Equal(Accept) {
					b.transaction.origRcptTos = append(b.transaction.origRcptTos, addr.NewRcptTo(rcptTo, esmtpArgs, m.Get(milter.MacroRcptMailer)))
					return milter.RespContinue, nil
				} else {
					if r.Decision == Discard {
						b.transaction.hasDecision = true
						b.transaction.decision = Discard
					}
					return decisionToResponse(r.Decision), nil
				}
			case <-ticker.C:
				err := m.Progress()
				if err != nil && !errors.Is(err, milter.ErrVersionTooLow) {
					// the connection broke, instruct validator function to abort
					cancel()
					// wait for validator function
					r := <-done
					// if there was no error in the validator function (e.g. it did not actually check ctx.Done())
					// return the context error (it is non-nil at this point)
					if r.Err == nil {
						return b.error(ctx.Err())
					}
					return b.error(r.Err)
				}
			}
		}
	} else {
		b.transaction.origRcptTos = append(b.transaction.origRcptTos, addr.NewRcptTo(rcptTo, esmtpArgs, m.Get(milter.MacroRcptMailer)))
		return milter.RespContinue, nil
	}
}

func (b *backend) Data(m milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	b.transaction.queueId = m.Get(milter.MacroQueueId)
	return b.decideOrContinue(DecisionAtData, m)
}

func (b *backend) Header(name string, value string, m milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		if m.Protocol()&milter.OptSkip != 0 {
			return milter.RespSkip, nil
		}
		return milter.RespContinue, nil
	}
	b.headerCount++
	if b.headerCount > uint64(b.opts.header.Max) {
		switch b.opts.header.MaxAction {
		case RejectMessageWhenTooBig:
			return milter.RejectWithCodeAndReason(552, fmt.Sprintf("5.3.4 Maximum allowed header lines (%d) exceeded.", b.opts.header.Max))
		case ClearWhenTooBig:
			if b.transaction.origHeaders != nil && b.transaction.origHeaders.Len() > 0 {
				b.transaction.origHeaders = &header.Header{}
			}
			return milter.RespContinue, nil
		default:
			return milter.RespContinue, nil
		}
	}
	name = strings.Trim(name, " \t\r\n")
	// the MTA told us in the negotiation packet that it swallows the space after the colon.
	if !b.leadingSpace {
		// we only add a space when the first character is not a tab - sendmail swallows the first space
		// we add it back to re-construct the raw header as it was sent by the client
		if len(value) == 0 || value[0] != '\t' {
			value = " " + value
		}
	}
	if name == "" {
		milter.LogWarning("skip header because we got an empty name")
	} else {
		b.transaction.addHeader(name, []byte(fmt.Sprintf("%s:%s", name, value)))
	}
	return milter.RespContinue, nil
}

func (b *backend) Headers(m milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	return b.decideOrContinue(DecisionAtEndOfHeaders, m)
}

func (b *backend) BodyChunk(chunk []byte, m milter.Modifier) (*milter.Response, error) {
	if !b.transaction.wantsBodyChunks() {
		if m.Protocol()&milter.OptSkip != 0 {
			return milter.RespSkip, nil
		}
		return milter.RespContinue, nil
	}
	err := b.transaction.addBodyChunk(chunk)
	if err != nil {
		if errors.Is(err, body.ErrBodyTooLarge) && b.opts.body.MaxAction == RejectMessageWhenTooBig {
			return milter.RejectWithCodeAndReason(552, fmt.Sprintf("5.3.4 Maximum allowed body size of %d bytes exceeded.", b.opts.body.MaxSize))
		}
		return b.error(err)
	}
	return milter.RespContinue, nil
}

func (b *backend) readyForNewMessage() {
	if b.transaction != nil {
		connect, helo := b.transaction.connect, b.transaction.helo
		b.Cleanup(nil)
		b.transaction.connect, b.transaction.helo = connect, helo
	} else {
		b.Cleanup(nil)
	}
}

func (b *backend) EndOfMessage(m milter.Modifier) (*milter.Response, error) {
	if !b.transaction.hasDecision && b.transaction.queueId == "" {
		b.transaction.queueId = m.Get(milter.MacroQueueId)
	}
	if !b.transaction.hasDecision {
		b.makeDecision(m)
	}

	if b.transaction.decisionErr != nil {
		return b.error(b.transaction.decisionErr)
	}

	if err := b.transaction.sendModifications(m); err != nil {
		return b.error(err)
	}

	response := b.transaction.response()

	b.readyForNewMessage()

	return response, nil
}

func (b *backend) Abort(_ milter.Modifier) error {
	b.readyForNewMessage()
	return nil
}

func (b *backend) Cleanup(_ milter.Modifier) {
	if b.transaction != nil {
		b.transaction.cleanup()
		b.transaction = nil
	}
	b.headerCount = 0
	b.transaction = &transaction{bodyOpt: *b.opts.body}
}

var _ milter.Milter = (*backend)(nil)
