package mailfilter

import (
	"context"
	"fmt"
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
}

func (b *backend) decideOrContinue(stage DecisionAt, m *milter.Modifier) (*milter.Response, error) {
	if b.opts.decisionAt == stage {
		b.makeDecision(m)
		if !b.transaction.hasModifications() {
			if b.transaction.decisionErr != nil {
				return b.error(b.transaction.decisionErr)
			}
			return b.transaction.response(), nil
		}
	}
	return milter.RespContinue, nil
}

func (b *backend) error(err error) (*milter.Response, error) {
	b.Cleanup()
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

func (b *backend) makeDecision(m *milter.Modifier) {
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
			if err != nil {
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

func (b *backend) Connect(host string, family string, port uint16, addr string, m *milter.Modifier) (*milter.Response, error) {
	b.Cleanup()
	b.transaction.mta = MTA{
		Version: m.Macros.Get(milter.MacroMTAVersion),
		FQDN:    m.Macros.Get(milter.MacroMTAFQDN),
		Daemon:  m.Macros.Get(milter.MacroDaemonName),
	}
	b.transaction.connect = Connect{
		Host:   host,
		Family: family,
		Port:   port,
		Addr:   addr,
		IfName: m.Macros.Get(milter.MacroIfName),
		IfAddr: m.Macros.Get(milter.MacroIfAddr),
	}
	return b.decideOrContinue(DecisionAtConnect, m)
}

func (b *backend) Helo(name string, m *milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	b.transaction.helo = Helo{
		Name:        name,
		TlsVersion:  m.Macros.Get(milter.MacroTlsVersion),
		Cipher:      m.Macros.Get(milter.MacroCipher),
		CipherBits:  m.Macros.Get(milter.MacroCipherBits),
		CertSubject: m.Macros.Get(milter.MacroCertSubject),
		CertIssuer:  m.Macros.Get(milter.MacroCertIssuer),
	}
	return b.decideOrContinue(DecisionAtHelo, m)
}

func (b *backend) MailFrom(from string, esmtpArgs string, m *milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	b.transaction.origMailFrom = addr.NewMailFrom(from, esmtpArgs, m.Macros.Get(milter.MacroMailMailer), m.Macros.Get(milter.MacroAuthAuthen), m.Macros.Get(milter.MacroAuthType))
	return b.decideOrContinue(DecisionAtMailFrom, m)
}

func (b *backend) RcptTo(rcptTo string, esmtpArgs string, m *milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespSkip, nil
	}
	b.transaction.origRcptTos = append(b.transaction.origRcptTos, addr.NewRcptTo(rcptTo, esmtpArgs, m.Macros.Get(milter.MacroRcptMailer)))
	return milter.RespContinue, nil
}

func (b *backend) Data(m *milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	b.transaction.queueId = m.Macros.Get(milter.MacroQueueId)
	return b.decideOrContinue(DecisionAtData, m)
}

func (b *backend) Header(name string, value string, _ *milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespSkip, nil
	}
	name = strings.Trim(name, " \t\r\n")
	if b.leadingSpace {
		// the MTA did not actually *not* swallow the space, so we add a space because it is required
		if len(value) > 0 && value[0] != ' ' && value[0] != '\t' {
			value = " " + value
		}
	} else {
		// we only add a space when the first character is not a tab - sendmail swallows the first space
		if len(value) == 0 || value[0] != '\t' {
			value = " " + value
		}
	}
	if value == "" {
		value = " "
	}
	if name == "" {
		milter.LogWarning("skip header because we got an empty  name")
	} else {
		b.transaction.addHeader(name, []byte(fmt.Sprintf("%s:%s", name, value)))
	}
	return milter.RespContinue, nil
}

func (b *backend) Headers(m *milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision {
		return milter.RespContinue, nil
	}
	return b.decideOrContinue(DecisionAtEndOfHeaders, m)
}

func (b *backend) BodyChunk(chunk []byte, _ *milter.Modifier) (*milter.Response, error) {
	if b.transaction.hasDecision || b.opts.skipBody {
		return milter.RespSkip, nil
	}
	err := b.transaction.addBodyChunk(chunk)
	if err != nil {
		return b.error(err)
	}
	return milter.RespContinue, nil
}

func (b *backend) readyForNewMessage() {
	if b.transaction != nil {
		connect, helo := b.transaction.connect, b.transaction.helo
		b.Cleanup()
		b.transaction.connect, b.transaction.helo = connect, helo
	} else {
		b.Cleanup()
	}
}

func (b *backend) EndOfMessage(m *milter.Modifier) (*milter.Response, error) {
	if !b.transaction.hasDecision && b.transaction.queueId == "" {
		b.transaction.queueId = m.Macros.Get(milter.MacroQueueId)
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

func (b *backend) Abort(_ *milter.Modifier) error {
	b.readyForNewMessage()
	return nil
}

func (b *backend) Cleanup() {
	if b.transaction != nil {
		b.transaction.cleanup()
	}
	b.transaction = &transaction{}
}

var _ milter.Milter = (*backend)(nil)
