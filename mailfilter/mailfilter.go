// Package mailfilter allows you to write milter filters without boilerplate code
package mailfilter

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/d--j/go-milter"
)

// DecisionModificationFunc is the callback function that you need to implement to create a mail filter.
//
// ctx is a [context.Context] that might get canceled when the connection to the MTA fails while your callback is running.
// If your decision function is running longer than one second the [MailFilter] automatically sends progress notifications
// every second so that MTA does not time out the milter connection.
//
// trx is the [Trx] object that you can inspect to see what the [MailFilter] got as information about the current SMTP transaction.
// You can also use trx to modify the transaction (e.g. change recipients, alter headers).
//
// decision is your [Decision] about this SMTP transaction. Use [Accept], [TempFail], [Reject], [Discard], [QuarantineResponse], or [CustomErrorResponse].
//
// If you return a non-nil error [WithErrorHandling] will determine what happens with the current SMTP transaction.
type DecisionModificationFunc func(ctx context.Context, trx Trx) (decision Decision, err error)

type MailFilter struct {
	wgDone   sync.WaitGroup
	socket   net.Listener
	server   *milter.Server
	options  options
	protocol milter.OptProtocol
}

// New creates and starts a new [MailFilter] with a socket listening on network and address.
// decision is the callback that should implement the filter logic.
// opts are optional [Option] functions that configure/fine-tune the mail filter.
func New(network, address string, decision DecisionModificationFunc, opts ...Option) (*MailFilter, error) {
	resolvedOptions := options{
		decisionAt:    DecisionAtEndOfMessage,
		errorHandling: TempFailWhenError,
	}

	for _, o := range opts {
		o(&resolvedOptions)
	}

	if resolvedOptions.body == nil {
		resolvedOptions.body = &bodyOption{
			MaxMem:    200 * 1024,        // 200 KiB
			MaxSize:   1024 * 1024 * 100, // 100 MiB
			MaxAction: TruncateWhenTooBig,
		}
	} else {
		if !resolvedOptions.body.Skip {
			if resolvedOptions.body.MaxSize <= 0 {
				return nil, fmt.Errorf("the parameter maxSize of WithBody must be positive")
			}
			if resolvedOptions.body.MaxAction != RejectMessageWhenTooBig && resolvedOptions.body.MaxAction != ClearWhenTooBig && resolvedOptions.body.MaxAction != TruncateWhenTooBig {
				return nil, fmt.Errorf("the parameter maxAction of WithBody is invalid")
			}
			if resolvedOptions.body.MaxMem < 0 {
				return nil, fmt.Errorf("the parameter maxMem of WithBody cannot be negative")
			}
		}
	}
	if resolvedOptions.header == nil {
		resolvedOptions.header = &headerOption{
			Max:       512,
			MaxAction: TruncateWhenTooBig,
		}
	} else {
		if resolvedOptions.header.Max == 0 {
			return nil, fmt.Errorf("the parameter maxHeaders of WithHeader must be positive")
		}
		if resolvedOptions.header.MaxAction != RejectMessageWhenTooBig && resolvedOptions.header.MaxAction != ClearWhenTooBig && resolvedOptions.header.MaxAction != TruncateWhenTooBig {
			return nil, fmt.Errorf("the parameter maxAction of WithHeader is invalid")
		}
	}
	switch resolvedOptions.errorHandling {
	case TempFailWhenError, RejectWhenError, AcceptWhenError, Error:
		// nothing to do
	default:
		return nil, fmt.Errorf("the parameter errorHandling of WithErrorHandling is invalid")
	}

	actions := milter.AllClientSupportedActionMasks
	protocol := milter.OptHeaderLeadingSpace | milter.OptNoUnknown

	switch resolvedOptions.decisionAt {
	case DecisionAtConnect:
		protocol = protocol | milter.OptNoHelo | milter.OptNoMailFrom | milter.OptNoRcptTo | milter.OptNoData | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody
	case DecisionAtHelo:
		protocol = protocol | milter.OptNoConnReply | milter.OptNoMailFrom | milter.OptNoRcptTo | milter.OptNoData | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody
	case DecisionAtMailFrom:
		protocol = protocol | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoRcptTo | milter.OptNoData | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody
	case DecisionAtData:
		protocol = protocol | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoMailReply | milter.OptNoRcptReply | milter.OptNoHeaders | milter.OptNoEOH | milter.OptNoBody
	case DecisionAtEndOfHeaders:
		protocol = protocol | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoMailReply | milter.OptNoRcptReply | milter.OptNoDataReply | milter.OptNoHeaderReply | milter.OptNoBody
	default:
		protocol = protocol | milter.OptNoConnReply | milter.OptNoHeloReply | milter.OptNoRcptReply | milter.OptNoHeaderReply | milter.OptNoEOHReply | milter.OptNoBodyReply
	}
	if resolvedOptions.body.Skip {
		protocol = protocol | milter.OptNoBody
	}
	macroStages := make([][]milter.MacroName, 0, 6)
	macroStages = append(macroStages, []milter.MacroName{milter.MacroIfName, milter.MacroIfAddr, milter.MacroMTAVersion, milter.MacroMTAFQDN, milter.MacroDaemonName}) // StageConnect
	if resolvedOptions.decisionAt > DecisionAtConnect {
		// StageHelo
		macroStages = append(macroStages, []milter.MacroName{milter.MacroTlsVersion, milter.MacroCipher, milter.MacroCipherBits, milter.MacroCertSubject, milter.MacroCertIssuer})
	}
	if resolvedOptions.decisionAt > DecisionAtHelo { // StageMail
		macroStages = append(macroStages, []milter.MacroName{milter.MacroMailMailer, milter.MacroAuthAuthen, milter.MacroAuthType})
	}
	if resolvedOptions.decisionAt > DecisionAtMailFrom {
		macroStages = append(macroStages, []milter.MacroName{milter.MacroRcptMailer}) // StageRcpt
		// try two different stages to get the queue ID, normally at the beginning of the DATA command it is already assigned
		// but if it is not, try at the end of the message
		macroStages = append(macroStages, []milter.MacroName{milter.MacroQueueId}) //StageData
		macroStages = append(macroStages, []milter.MacroName{milter.MacroQueueId}) //StageEOM
		macroStages = append(macroStages, []milter.MacroName{})                    //StageEOH
	}

	milterOptions := []milter.Option{
		milter.WithDynamicMilter(func(version uint32, action milter.OptAction, protocol milter.OptProtocol, maxData milter.DataSize) milter.Milter {
			return &backend{
				opts:         resolvedOptions,
				decision:     decision,
				leadingSpace: protocol&milter.OptHeaderLeadingSpace != 0,
				transaction:  &transaction{bodyOpt: *resolvedOptions.body},
			}
		}),
		milter.WithActions(actions),
		milter.WithProtocols(protocol),
	}
	for i, macros := range macroStages {
		milterOptions = append(milterOptions, milter.WithMacroRequest(milter.MacroStage(i), macros))
	}

	// create socket to listen on
	socket, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}

	// create server with assembled options
	server := milter.NewServer(milterOptions...)

	f := &MailFilter{
		socket:   socket,
		server:   server,
		options:  resolvedOptions,
		protocol: protocol,
	}

	// start the milter
	f.wgDone.Add(1)
	go func(socket net.Listener) {
		if err := server.Serve(socket); err != nil {
			milter.LogWarning("server.Server() error: %s", err)
		}
		f.wgDone.Done()
	}(socket)

	return f, nil
}

// Addr returns the [net.Addr] of the listening socket of this [MailFilter].
// This method returns nil when the socket is not set.
func (f *MailFilter) Addr() net.Addr {
	if f.socket == nil {
		return nil
	}
	return f.socket.Addr()
}

// Wait waits for the end of the [MailFilter] server.
func (f *MailFilter) Wait() {
	f.wgDone.Wait()
	_ = f.server.Close()
}

// Close stops the [MailFilter] server.
func (f *MailFilter) Close() {
	_ = f.server.Close()
}

// Shutdown gracefully stops the [MailFilter] server.
func (f *MailFilter) Shutdown(ctx context.Context) error {
	return f.server.Shutdown(ctx)
}
