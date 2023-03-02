package milter

import (
	"time"
)

// NewMilterFunc is the signature of a function that can be used with [WithDynamicMilter] to configure the [Milter] backend.
// The parameters version, action, protocol and maxData are the negotiated values.
type NewMilterFunc func(version uint32, action OptAction, protocol OptProtocol, maxData DataSize) Milter

// NegotiationCallbackFunc is the signature of a [WithNegotiationCallback] function.
// With this callback function you can override the negotiation process.
type NegotiationCallbackFunc func(mtaVersion, milterVersion uint32, mtaActions, milterActions OptAction, mtaProtocol, milterProtocol OptProtocol, offeredDataSize DataSize) (version uint32, actions OptAction, protocol OptProtocol, maxDataSize DataSize, err error)

type options struct {
	maxVersion                  uint32
	actions                     OptAction
	protocol                    OptProtocol
	dialer                      Dialer
	readTimeout, writeTimeout   time.Duration
	offeredMaxData, usedMaxData DataSize
	macrosByStage               macroRequests
	newMilter                   NewMilterFunc
	negotiationCallback         NegotiationCallbackFunc
}

// Option can be used to configure [Client] and [Server].
type Option func(*options)

// WithAction adds action to the actions your MTA supports or your [Milter] needs. You need to specify this since this library cannot
// guess what your MTA can handle or your milter needs.
// 0 is a valid value when your MTA does not support any message modification (only rejection) or your milter does not need any message modifications.
func WithAction(action OptAction) Option {
	return func(h *options) {
		h.actions = h.actions | action
	}
}

// WithoutAction removes action from the list of actions this MTA supports/[Milter] needs.
func WithoutAction(action OptAction) Option {
	return func(h *options) {
		h.actions = h.actions & ^action
	}
}

// WithActions sets the actions your MTA supports or your [Milter] needs. You need to specify this since this library cannot
// guess what your MTA can handle or your milter needs.
// 0 is a valid value when your MTA does not support any message modification (only rejection) or your milter does not need any message modifications.
func WithActions(actions OptAction) Option {
	return func(h *options) {
		h.actions = actions
	}
}

// WithProtocol adds protocol to the protocol features your MTA should be able to handle or your [Milter] needs.
// For MTAs you can normally skip setting this option since we then just default to all protocol feature that this library supports.
// [Milter] should specify this option to instruct the MTA to not send any events that your [Milter] does not need or to not expect any response from events that you are not using to accept or reject an SMTP transaction.
func WithProtocol(protocol OptProtocol) Option {
	return func(h *options) {
		h.protocol = h.protocol | protocol
	}
}

// WithoutProtocol removes protocol from the list of protocol features this MTA supports/[Milter] requests.
func WithoutProtocol(protocol OptProtocol) Option {
	return func(h *options) {
		h.protocol = h.protocol & ^protocol
	}
}

// WithProtocols sets the protocol features your MTA should be able to handle or your [Milter] needs.
// For MTAs you can normally skip setting this option since we then just default to all protocol feature that this library supports.
// Milter should specify this option to instruct the MTA to not send any events that your [Milter] does not need or to not expect any response from events that you are not using to accept or reject an SMTP transaction.
func WithProtocols(protocol OptProtocol) Option {
	return func(h *options) {
		h.protocol = protocol
	}
}

// WithMaximumVersion sets the maximum milter version your MTA or milter filter accepts.
// The default is to use the maximum supported version.
func WithMaximumVersion(version uint32) Option {
	return func(h *options) {
		h.maxVersion = version
	}
}

// WithDialer sets the [net.Dialer] this [Client] will use. You can use this to e.g. set the connection timeout of the client.
// The default is to use a [net.Dialer] with a connection timeout of 10 seconds.
func WithDialer(dialer Dialer) Option {
	return func(h *options) {
		h.dialer = dialer
	}
}

// WithReadTimeout sets the read-timeout for all read operations of this [Client] or [Server].
// The default is a read-timeout of 10 seconds.
func WithReadTimeout(timeout time.Duration) Option {
	return func(h *options) {
		h.readTimeout = timeout
	}
}

// WithWriteTimeout sets the write-timeout for all read operations of this [Client] or [Server].
// The default is a write-timeout of 10 seconds.
func WithWriteTimeout(timeout time.Duration) Option {
	return func(h *options) {
		h.writeTimeout = timeout
	}
}

// WithOfferedMaxData sets the [DataSize] that your MTA wants to offer to milters.
// The milter needs to accept this offer in protocol negotiation for it to become effective.
// This is just an indication to the milter that it can send bigger packages.
// This library does not care what value was negotiated and always accept packages of up to 512 MB.
//
// This is a [Client] only [Option].
func WithOfferedMaxData(offeredMaxData DataSize) Option {
	return func(h *options) {
		h.offeredMaxData = offeredMaxData
	}
}

// WithUsedMaxData sets the [DataSize] that your MTA or milter uses to send packages to the other party.
// The default value is [DataSize64K] for maximum compatibility.
// If you set this to 0 the [Client] will use the value of [WithOfferedMaxData] and the [Server] will use the dataSize that it
// negotiated with the MTA.
//
// Setting the maximum used data size to something different might trigger the other party to an error.
// MTAs like Postfix/sendmail and newer libmilter versions can handle bigger values without negotiation.
// E.g. Postfix will accept packets of up to 2 GB. This library has a hard maximum packet size of 512 MB.
func WithUsedMaxData(usedMaxData DataSize) Option {
	return func(h *options) {
		h.usedMaxData = usedMaxData
	}
}

// WithoutDefaultMacros deletes all macro stage definitions that were made before this [Option].
// Use it in [NewClient] do not use the default. Since [NewServer] does not have a default, it is a no-op in [NewServer].
func WithoutDefaultMacros() Option {
	return func(h *options) {
		h.macrosByStage = nil
	}
}

// WithMacroRequest defines the macros that your [Client] intends to send at stage, or it instructs the [Server] to ask for these macros at this stage.
//
// For [Client]: The milter can request other macros at protocol negotiation but if it does not do this (most do not) it will receive these macros at these stages.
//
// For [Server]: MTAs like sendmail and Postfix honor your macro requests and only send you the macros you requested (even if other macros were configured in their configuration).
// If it is possible your milter should gracefully handle the case that the MTA does not honor your macro requests.
// This function automatically sets the action [OptSetMacros]
func WithMacroRequest(stage MacroStage, macros []MacroName) Option {
	return func(h *options) {
		if h.macrosByStage == nil {
			h.macrosByStage = make([][]MacroName, StageEndMarker)
		}
		h.macrosByStage[stage] = macros
	}
}

// WithMilter sets the [Milter] backend this [Server] uses.
//
// This is a [Server] only [Option].
func WithMilter(newMilter func() Milter) Option {
	return func(h *options) {
		h.newMilter = func(uint32, OptAction, OptProtocol, DataSize) Milter {
			return newMilter()
		}
	}
}

// WithDynamicMilter sets the [Milter] backend this [Server] uses.
// This [Option] sets the milter with the negotiated version, action and protocol.
// You can use this to dynamically configure the [Milter] backend.
//
// This is a [Server] only [Option].
func WithDynamicMilter(newMilter NewMilterFunc) Option {
	return func(h *options) {
		h.newMilter = newMilter
	}
}

// WithNegotiationCallback is an expert [Option] with which you can overwrite the negotiation process.
//
// You should not need to use this. You might easily break things. You are responsible to adhere to
// the milter protocol negotiation rules (they unfortunately only exist in sendmail & libmilter source code).
//
// This is a [Server] only [Option].
func WithNegotiationCallback(negotiationCallback NegotiationCallbackFunc) Option {
	return func(h *options) {
		h.negotiationCallback = negotiationCallback
	}
}
