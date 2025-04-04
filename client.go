package milter

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/d--j/go-milter/internal/wire"
	"github.com/d--j/go-milter/milterutil"
	"github.com/emersion/go-message/textproto"
)

// MaxClientProtocolVersion is the maximum Milter protocol version implemented by the client.
const MaxClientProtocolVersion uint32 = 6

const allClientSupportedProtocolMasks = OptNoConnect | OptNoHelo | OptNoMailFrom | OptNoRcptTo | OptNoBody | OptNoHeaders | OptNoEOH | OptNoUnknown | OptNoData | OptSkip | OptRcptRej | OptNoHeaderReply | OptNoConnReply | OptNoHeloReply | OptNoMailReply | OptNoRcptReply | OptNoDataReply | OptNoUnknownReply | OptNoEOHReply | OptNoBodyReply | OptHeaderLeadingSpace // SMFI_CURR_PROT
const allClientSupportedProtocolMasksV2 = OptNoConnect | OptNoHelo | OptNoMailFrom | OptNoRcptTo | OptNoBody | OptNoHeaders | OptNoEOH                                                                                                                                                                                                                                      // SMFI_V2_PROT
const allClientSupportedProtocolMasksV3 = allClientSupportedProtocolMasksV2 | OptNoUnknown
const allClientSupportedProtocolMasksV4 = allClientSupportedProtocolMasksV3 | OptNoData

const AllClientSupportedActionMasks = OptAddHeader | OptChangeBody | OptAddRcpt | OptRemoveRcpt | OptChangeHeader | OptQuarantine | OptChangeFrom | OptAddRcptWithArgs | OptSetMacros
const allClientSupportedActionMasksV2 = OptAddHeader | OptChangeBody | OptAddRcpt | OptRemoveRcpt | OptChangeHeader | OptQuarantine

// Dialer is the interface of the only method we use of a net.Dialer.
type Dialer interface {
	Dial(network string, addr string) (net.Conn, error)
}

// Client is a wrapper for managing milter connections to one milter.
//
// You need to call Session to actually open a connection to the milter.
type Client struct {
	options options
	network string
	address string
}

// NewClient creates a new Client object connection to a miter at network / address.
// If you do not specify any opts the defaults are:
//
// It uses 10 seconds for connection/read/write timeouts and allows milter to
// send any actions supported by library.
//
// You generally want to use WithAction to advertise to the milter what modification options your MTA supports.
// A value of 0 is a valid value –then your MTA only supports accepting or rejecting an SMTP transaction.
//
// If WithDialer is not used, a net.Dialer with 10 seconds connection timeout will be used.
// If WithMaximumVersion is not used, MaxClientProtocolVersion will be used.
// If WithProtocol or WithProtocols is not set, it defaults to all protocol features the library can handle for the specified maximum milter version.
// If WithOfferedMaxData is not used, DataSize64K will be used.
// If WithoutDefaultMacros or WithMacroRequest are not used the following default macro stages are used:
//
//	WithMacroRequest(StageConnect, []MacroName{MacroMTAFQDN, MacroDaemonName, MacroIfName, MacroIfAddr})
//	WithMacroRequest(StageHelo, []MacroName{MacroTlsVersion, MacroCipher, MacroCipherBits, MacroCertSubject, MacroCertIssuer})
//	WithMacroRequest(StageMail, []MacroName{MacroAuthType, MacroAuthAuthen, MacroAuthSsf, MacroAuthAuthor, MacroMailMailer, MacroMailHost, MacroMailAddr})
//	WithMacroRequest(StageRcpt, []MacroName{MacroRcptMailer, MacroRcptHost, MacroRcptAddr})
//	WithMacroRequest(StageEOM, []MacroName{MacroQueueId})
//
// This function will panic when you provide invalid options.
func NewClient(network, address string, opts ...Option) *Client {
	options := options{
		dialer: &net.Dialer{
			Timeout: 10 * time.Second,
		},
		readTimeout:    10 * time.Second,
		writeTimeout:   10 * time.Second,
		maxVersion:     MaxClientProtocolVersion,
		actions:        AllClientSupportedActionMasks,
		protocol:       allClientSupportedProtocolMasks,
		offeredMaxData: DataSize64K,
		usedMaxData:    DataSize64K,
		macrosByStage: [][]MacroName{
			{MacroMTAFQDN, MacroDaemonName, MacroIfName, MacroIfAddr},                                                      // StageConnect
			{MacroTlsVersion, MacroCipher, MacroCipherBits, MacroCertSubject, MacroCertIssuer},                             // StageHelo
			{MacroAuthType, MacroAuthAuthen, MacroAuthSsf, MacroAuthAuthor, MacroMailMailer, MacroMailHost, MacroMailAddr}, // StageMail
			{MacroRcptMailer, MacroRcptHost, MacroRcptAddr},                                                                // StageRcpt
			{},             // StageData
			{MacroQueueId}, // StageEOM
			{},             // StageEOH
		},
	}
	if len(opts) > 0 {
		for _, o := range opts {
			if o != nil {
				o(&options)
			}
		}
	}

	if options.dialer == nil {
		panic("milter: you cannot pass <nil> to WithDialer")
	}
	if options.maxVersion > MaxClientProtocolVersion || options.maxVersion == 1 {
		panic("milter: this library cannot handle this milter version")
	}
	if options.offeredMaxData != DataSize64K && options.offeredMaxData != DataSize256K && options.offeredMaxData != DataSize1M {
		panic("milter: wrong data size passed to WithOfferedMaxData")
	}
	// ensure we only offer protocol options the version can handel
	if options.protocol != 0 {
		var all OptProtocol
		switch options.maxVersion {
		case 2:
			all = allClientSupportedProtocolMasksV2
		case 3:
			all = allClientSupportedProtocolMasksV3
		case 4:
			all = allClientSupportedProtocolMasksV4
		default:
			all = allClientSupportedProtocolMasks
		}
		if options.protocol&^all != 0 {
			panic(fmt.Sprintf("Provided invalid protocol options for milter version %d %q", options.maxVersion, options.protocol))
		}
	}
	// offering nothing to filters is unlikely, just default to all we can handle
	if options.protocol == 0 {
		switch options.maxVersion {
		case 2:
			options.protocol = allClientSupportedProtocolMasksV2
		case 3:
			options.protocol = allClientSupportedProtocolMasksV3
		case 4, 5:
			options.protocol = allClientSupportedProtocolMasksV4
		default:
			options.protocol = allClientSupportedProtocolMasks
		}
	}
	if options.newMilter != nil {
		panic("milter: WithMilter/WithDynamicMilter is a server only option")
	}
	if options.negotiationCallback != nil {
		panic("milter: WithNegotiationCallback is a server only option")
	}

	return &Client{
		options: options,
		network: network,
		address: address,
	}
}

// String returns the network and address that his Client is configured to connect to.
// This method is go-routine save.
func (c *Client) String() string {
	return fmt.Sprintf("%s:%s", c.network, c.address)
}

// Session opens a new connection to this milter and negotiates protocol features with it.
//
// The macros parameter defines the Macros this ClientSession will use to send to the milter.
// It can be nil then this session will not send any macros to the milter.
// Set macro values as soon as you know them (e.g. the MacroMTAFQDN macro can be set before calling Session).
// It is your responsibility to clear command specific macros like MacroRcptMailer after
// the command got executed (on all milters in a list of milters).
//
// This method is go-routine save.
func (c *Client) Session(macros Macros) (*ClientSession, error) {
	conn, err := c.options.dialer.Dial(c.network, c.address)
	if err != nil {
		return nil, fmt.Errorf("milter: session create: %w", err)
	}

	return c.session(conn, macros)
}

func (c *Client) session(conn net.Conn, macros Macros) (*ClientSession, error) {
	s := &ClientSession{
		readTimeout:    c.options.readTimeout,
		writeTimeout:   c.options.writeTimeout,
		state:          clientStateClosed,
		macros:         macros,
		macrosByStages: make([][]string, StageEndMarker),
		maxBodySize:    uint32(c.options.usedMaxData),
	}
	if c.options.macrosByStage != nil {
		copy(s.macrosByStages, c.options.macrosByStage)
	}

	s.state = clientStateNegotiated

	s.conn = conn
	if err := s.negotiate(c.options.maxVersion, c.options.actions, c.options.protocol, c.options.offeredMaxData); err != nil {
		return nil, err
	}

	return s, nil
}

type clientSessionState uint32

const (
	clientStateClosed = iota
	clientStateNegotiated
	clientStateConnectCalled
	clientStateHeloCalled
	clientStateMailCalled
	clientStateRcptCalled
	clientStateDataCalled
	clientStateHeaderFieldCalled
	clientStateHeaderEndCalled
	clientStateBodyChunkCalled
	clientStateError
)

// ClientSession is a connection to one Client for one SMTP connection.
type ClientSession struct {
	conn net.Conn

	// negotiated version of this session
	version uint32

	// Bitmask of negotiated action options.
	actionOpts OptAction

	// Bitmask of negotiated protocol options.
	protocolOpts OptProtocol

	maxBodySize        uint32
	negotiatedBodySize uint32

	state       clientSessionState
	skip        bool
	skipUnknown bool
	closedErr   error

	readTimeout  time.Duration
	writeTimeout time.Duration

	macros         Macros
	macrosByStages [][]MacroName
}

func (s *ClientSession) errorOut(err error) error {
	s.state = clientStateError
	// close the connection
	if s.conn != nil {
		_ = s.conn.Close()
	}
	// give garbage collector a chance to free space
	s.macros = nil
	s.macrosByStages = nil
	return err
}

// negotiate exchanges OPTNEG messages with the milter and configures this session to the negotiated values.
func (s *ClientSession) negotiate(maximumVersion uint32, actionMask OptAction, protoMask OptProtocol, requestedMaxBuffer DataSize) error {
	// Send our mask, get mask from milter..
	msg := &wire.Message{
		Code: wire.CodeOptNeg,
		Data: make([]byte, 4*3),
	}
	binary.BigEndian.PutUint32(msg.Data, maximumVersion)
	binary.BigEndian.PutUint32(msg.Data[4:], uint32(actionMask))
	if requestedMaxBuffer == DataSize256K {
		binary.BigEndian.PutUint32(msg.Data[8:], uint32(protoMask)|optMds256K)
	} else if requestedMaxBuffer == DataSize1M {
		binary.BigEndian.PutUint32(msg.Data[8:], uint32(protoMask)|optMds1M)
	} else {
		binary.BigEndian.PutUint32(msg.Data[8:], uint32(protoMask))
	}

	if err := s.writePacket(msg); err != nil {
		return s.errorOut(fmt.Errorf("milter: negotiate: optneg write: %w", err))
	}
	msg, err := wire.ReadPacket(s.conn, s.readTimeout)
	if err != nil {
		return s.errorOut(fmt.Errorf("milter: negotiate: optneg read: %w", err))
	}
	if msg.Code != wire.CodeOptNeg {
		return s.errorOut(fmt.Errorf("milter: negotiate: unexpected code: %v", rune(msg.Code)))
	}
	if len(msg.Data) < 4*3 /* version + action mask + proto mask */ {
		return s.errorOut(fmt.Errorf("milter: negotiate: unexpected data size: %v", len(msg.Data)))
	}
	milterVersion := binary.BigEndian.Uint32(msg.Data[0:])

	if milterVersion < 2 || milterVersion > maximumVersion {
		return s.errorOut(fmt.Errorf("milter: negotiate: unsupported protocol version: %v", milterVersion))
	}

	s.version = milterVersion

	milterActionMask := OptAction(binary.BigEndian.Uint32(msg.Data[4:]))
	if milterActionMask&actionMask != milterActionMask {
		return s.errorOut(fmt.Errorf("milter: negotiate: unsupported actions requested: MTA %q filter %q", actionMask, milterActionMask))
	}
	s.actionOpts = milterActionMask
	milterProtoMask := OptProtocol(binary.BigEndian.Uint32(msg.Data[8:]))

	if uint32(milterProtoMask)&optMds1M == optMds1M {
		s.negotiatedBodySize = uint32(DataSize1M)
	} else if uint32(milterProtoMask)&optMds256K == optMds256K {
		s.negotiatedBodySize = uint32(DataSize256K)
	} else {
		s.negotiatedBodySize = uint32(DataSize64K)
	}

	// mask out the size flags
	milterProtoMask = milterProtoMask & (^OptProtocol(optInternal))
	if milterProtoMask&protoMask != milterProtoMask {
		return s.errorOut(fmt.Errorf("milter: negotiate: unsupported protocol options requested: MTA %q filter %q", protoMask, milterProtoMask))
	}

	// do not send commands that older versions do not understand
	if milterVersion <= 2 {
		milterProtoMask = milterProtoMask | OptNoUnknown
	}
	if milterVersion <= 3 {
		milterProtoMask = milterProtoMask | OptNoData
	}

	s.protocolOpts = milterProtoMask

	s.state = clientStateNegotiated

	// The filter defined macros it wants to get we only use them and not the defaults
	if len(msg.Data) > 4*4 {
		s.macrosByStages = make([][]string, StageEndMarker)
		l := len(msg.Data)
		offset := 4 * 3
		for l > offset+4 {
			stage := binary.BigEndian.Uint32(msg.Data[offset:])
			offset += 4
			requestedMacros := wire.ReadCString(msg.Data[offset:])
			offset += len(requestedMacros)
			if l <= offset || msg.Data[offset] != 0 {
				LogWarning("macros for stage %d are not null-terminated, skipping rest of list: %s", stage, requestedMacros)
				break
			}
			offset += 1 // skip null byte
			if stage < uint32(StageConnect) || stage >= uint32(StageEndMarker) {
				LogWarning("got request for unknown stage %d, ignoring this entry", stage)
				continue
			}
			if s.macrosByStages[MacroStage(stage)] != nil {
				LogWarning("macros for stage %d were send multiple times: %q is overwriting %q", stage, requestedMacros, strings.Join(s.macrosByStages[MacroStage(stage)], " "))
			}
			s.macrosByStages[MacroStage(stage)] = parseRequestedMacros(requestedMacros)
		}
	}
	for i := range s.macrosByStages {
		if s.macrosByStages[i] != nil {
			s.macrosByStages[i] = removeDuplicates(s.macrosByStages[i])
		}
	}

	return nil
}

// ProtocolOption checks whether the option is set in negotiated options.
func (s *ClientSession) ProtocolOption(opt OptProtocol) bool {
	return s.protocolOpts&opt != 0
}

// ActionOption checks whether the option is set in negotiated options.
func (s *ClientSession) ActionOption(opt OptAction) bool {
	return s.actionOpts&opt != 0
}

func (s *ClientSession) sendMacros(code wire.Code, names []MacroName) error {
	if s.macros == nil {
		return nil
	}
	msg := &wire.Message{
		Code: wire.CodeMacro,
		Data: []byte{byte(code)},
	}
	foundMacro := false
	for _, name := range names {
		// only send macros we actually defined
		if val, ok := s.macros.GetEx(name); ok {
			foundMacro = true
			msg.Data = wire.AppendCString(msg.Data, name)
			msg.Data = wire.AppendCString(msg.Data, val)
		}
	}
	// no need to send anything when we have not found a single macro
	if !foundMacro {
		return nil
	}

	if err := s.writePacket(msg); err != nil {
		return fmt.Errorf("milter: sendMacros: %w", err)
	}

	return nil
}

func (s *ClientSession) sendCmdMacros(code wire.Code, macros map[MacroName]string) error {
	if len(macros) == 0 {
		return nil
	}
	msg := &wire.Message{
		Code: wire.CodeMacro,
		Data: []byte{byte(code)},
	}
	for name, val := range macros {
		msg.Data = wire.AppendCString(msg.Data, name)
		msg.Data = wire.AppendCString(msg.Data, val)

	}

	if err := s.writePacket(msg); err != nil {
		return fmt.Errorf("milter: sendMacros: %w", err)
	}

	return nil
}

func (s *ClientSession) readAction(skipOk bool) (*Action, error) {
	for {
		msg, err := wire.ReadPacket(s.conn, s.readTimeout)
		if err != nil {
			return nil, s.errorOut(fmt.Errorf("action read: %w", err))
		}
		if wire.ActionCode(msg.Code) == wire.ActProgress /* progress */ {
			continue
		}

		act, err := parseAction(msg)
		if err != nil {
			return nil, err
		}
		if act.Type == ActionSkip && !skipOk {
			return nil, fmt.Errorf("action read: unexpected skip message received (can only be received after SMFIC_RCPT, SMFIC_HEADER, SMFIC_BODY when SMFIP_SKIP was negotiated)")
		}

		return act, err
	}
}

func (s *ClientSession) writePacket(msg *wire.Message) error {
	return wire.WritePacket(s.conn, msg, s.writeTimeout)
}

var actionContinue = &Action{Type: ActionContinue}

// Conn sends the connection information to the milter.
//
// It should be called once per milter session (from Session to Close).
// Exception: After you called Reset you need to call Conn again.
func (s *ClientSession) Conn(hostname string, family ProtoFamily, port uint16, addr string) (*Action, error) {
	if s.state != clientStateNegotiated {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}

	s.skip = false
	s.state = clientStateConnectCalled

	if len(s.macrosByStages) > int(StageConnect) && len(s.macrosByStages[StageConnect]) > 0 {
		if err := s.sendMacros(wire.CodeConn, s.macrosByStages[StageConnect]); err != nil {
			return nil, err
		}
	}

	if s.ProtocolOption(OptNoConnect) {
		return actionContinue, nil
	}

	msg := &wire.Message{
		Code: wire.CodeConn,
	}
	msg.Data = wire.AppendCString(msg.Data, hostname)
	msg.Data = append(msg.Data, byte(family))
	if family != FamilyUnknown {
		if family == FamilyInet || family == FamilyInet6 {
			msg.Data = wire.AppendUint16(msg.Data, port)
		} else if family == FamilyUnix {
			msg.Data = wire.AppendUint16(msg.Data, 0)
		}
		msg.Data = wire.AppendCString(msg.Data, addr)
	}

	if err := s.writePacket(msg); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: conn: %w", err))
	}

	if s.ProtocolOption(OptNoConnReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(false)
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: conn: %w", err))
	}

	if act.Type == ActionDiscard {
		LogWarning("Connect got a discard action, ignoring it")
		act.Type = ActionContinue
	}

	return act, nil
}

// Helo sends the HELO hostname to the milter.
//
// It should be called once per milter session (from Client.Session to Close).
func (s *ClientSession) Helo(helo string) (*Action, error) {
	if s.state != clientStateConnectCalled && s.state != clientStateHeloCalled {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}

	s.skip = false
	s.state = clientStateHeloCalled

	if len(s.macrosByStages) > int(StageHelo) && len(s.macrosByStages[StageHelo]) > 0 {
		if err := s.sendMacros(wire.CodeHelo, s.macrosByStages[StageHelo]); err != nil {
			return nil, s.errorOut(err)
		}
	}

	// Synthesise response as if server replied "go on" while in fact it does
	// not want or support that message.
	if s.ProtocolOption(OptNoHelo) {
		return actionContinue, nil
	}

	msg := &wire.Message{
		Code: wire.CodeHelo,
		Data: wire.AppendCString(nil, helo),
	}

	if err := s.writePacket(msg); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: helo: %w", err))
	}

	if s.ProtocolOption(OptNoHeloReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(false)
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: helo: %w", err))
	}

	if act.Type == ActionDiscard {
		LogWarning("Helo got a discard action, ignoring it")
		act.Type = ActionContinue
	}

	return act, nil
}

// Mail sends the sender (with optional esmtpArgs) to the milter.
func (s *ClientSession) Mail(sender string, esmtpArgs string) (*Action, error) {
	if s.state != clientStateHeloCalled {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}

	s.skip = false
	s.state = clientStateMailCalled

	if len(s.macrosByStages) > int(StageMail) && len(s.macrosByStages[StageMail]) > 0 {
		if err := s.sendMacros(wire.CodeMail, s.macrosByStages[StageMail]); err != nil {
			return nil, s.errorOut(err)
		}
	}

	if s.ProtocolOption(OptNoMailFrom) {
		return actionContinue, nil
	}

	msg := &wire.Message{
		Code: wire.CodeMail,
	}

	msg.Data = wire.AppendCString(msg.Data, "<"+sender+">")
	if len(esmtpArgs) > 0 {
		msg.Data = wire.AppendCString(msg.Data, esmtpArgs)
	}

	if err := s.writePacket(msg); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: mail: %w", err))
	}

	if s.ProtocolOption(OptNoMailReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(false)
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: mail: %w", err))
	}
	return act, nil
}

// Rcpt sends the RCPT TO rcpt (with optional esmtpArgs) to the milter.
// If s.ProtocolOption(OptRcptRej) is true the milter wants rejected recipients.
// The default is to only send valid recipients to the milter.
func (s *ClientSession) Rcpt(rcpt string, esmtpArgs string) (*Action, error) {
	if s.state != clientStateMailCalled && s.state != clientStateRcptCalled {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}
	if s.skip {
		return actionContinue, nil
	}

	s.state = clientStateRcptCalled

	if len(s.macrosByStages) > int(StageRcpt) && len(s.macrosByStages[StageRcpt]) > 0 {
		if err := s.sendMacros(wire.CodeRcpt, s.macrosByStages[StageRcpt]); err != nil {
			return nil, s.errorOut(err)
		}
	}

	if s.ProtocolOption(OptNoRcptTo) {
		return actionContinue, nil
	}

	msg := &wire.Message{
		Code: wire.CodeRcpt,
	}

	msg.Data = wire.AppendCString(msg.Data, "<"+rcpt+">")
	if len(esmtpArgs) > 0 {
		msg.Data = wire.AppendCString(msg.Data, esmtpArgs)
	}

	if err := s.writePacket(msg); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: rcpt: %w", err))
	}

	if s.ProtocolOption(OptNoRcptReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(s.ProtocolOption(OptSkip))
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: rcpt: %w", err))
	}
	if act.Type == ActionSkip {
		s.skip = true
		return actionContinue, nil
	}
	return act, nil
}

// DataStart sends the start of the DATA command to the milter.
// DataStart can be automatically called from Header, but you should normally call it explicitly.
//
// When your MTA can handle multiple milter in a chain, DataStart is the last event that is called individually for each milter in the chain.
// After DataStart you need to call the HeaderField/Header and BodyChunk&End/BodyReadFrom calls for the whole message serially to each milter.
// The first milter may alter the message and the next milter should receive the altered message, not the original message.
func (s *ClientSession) DataStart() (*Action, error) {
	if s.state != clientStateRcptCalled {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}
	s.skip = false
	s.state = clientStateDataCalled

	if s.version > 3 && len(s.macrosByStages) > int(StageData) && len(s.macrosByStages[StageData]) > 0 {
		if err := s.sendMacros(wire.CodeData, s.macrosByStages[StageData]); err != nil {
			return nil, s.errorOut(err)
		}
	}

	if s.ProtocolOption(OptNoData) {
		return actionContinue, nil
	}

	msg := &wire.Message{
		Code: wire.CodeData,
	}

	if err := s.writePacket(msg); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: rcpt: %w", err))
	}

	if s.ProtocolOption(OptNoDataReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(false)
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: rcpt: %w", err))
	}
	return act, nil
}

func trimLastLineBreak(in string) string {
	l := len(in)
	if l > 2 && in[l-2:] == "\r\n" {
		return in[:l-2]
	}
	if l > 1 && in[l-1:] == "\n" {
		return in[:l-1]
	}
	if l > 1 && in[l-1:] == "\r" {
		return in[:l-1]
	}
	return in
}

// HeaderField sends a single header field to the milter.
//
// Value should be the original field value without any unfolding applied.
// value may contain the last CR LF that ist the end marker of this header.
//
// HeaderEnd() must be called after the last field.
//
// You can send macros to the milter with macros. They only get send to the milter when it wants header values and it did not send a skip response.
// Thus, the macros you send here should be relevant to this header only.
func (s *ClientSession) HeaderField(key, value string, macros map[MacroName]string) (*Action, error) {
	if s.state > clientStateHeaderFieldCalled || s.state < clientStateDataCalled {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}
	if s.skip {
		return actionContinue, nil
	}

	s.state = clientStateHeaderFieldCalled

	if s.ProtocolOption(OptNoHeaders) {
		return actionContinue, nil
	}

	if err := s.sendCmdMacros(wire.CodeHeader, macros); err != nil {
		return nil, s.errorOut(err)
	}

	msg := &wire.Message{
		Code: wire.CodeHeader,
	}
	msg.Data = wire.AppendCString(msg.Data, key)
	msg.Data = wire.AppendCString(msg.Data, trimLastLineBreak(value))

	if err := s.writePacket(msg); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: header field: %w", err))
	}

	if s.ProtocolOption(OptNoHeaderReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(s.ProtocolOption(OptSkip))
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: header field: %w", err))
	}
	if act.Type == ActionSkip {
		s.skip = true
		return actionContinue, nil
	}
	return act, nil
}

// HeaderEnd send the EOH (End-Of-Header) message to the milter.
//
// No HeaderField calls are allowed after this point.
func (s *ClientSession) HeaderEnd() (*Action, error) {
	if s.state > clientStateHeaderFieldCalled || s.state < clientStateDataCalled {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}
	s.skip = false
	s.state = clientStateHeaderEndCalled

	if len(s.macrosByStages) > int(StageEOH) && len(s.macrosByStages[StageEOH]) > 0 {
		if err := s.sendMacros(wire.CodeEOH, s.macrosByStages[StageEOH]); err != nil {
			return nil, s.errorOut(err)
		}
	}

	if s.ProtocolOption(OptNoEOH) {
		return actionContinue, nil
	}

	if err := s.writePacket(&wire.Message{
		Code: wire.CodeEOH,
	}); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: header end: %w", err))
	}

	if s.ProtocolOption(OptNoEOHReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(false)
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: header end: %w", err))
	}
	return act, nil
}

// Header sends each field from textproto.Header followed by EOH unless
// header messages are disabled during negotiation.
//
// You may call HeaderField before calling this method but since it calls HeaderEnd afterward
// you should call BodyChunk or BodyReadFrom.
func (s *ClientSession) Header(hdr textproto.Header) (*Action, error) {
	if s.state < clientStateRcptCalled || s.state > clientStateHeaderFieldCalled {
		return nil, s.errorOut(fmt.Errorf("milter: in wrong state %d", s.state))
	}
	if s.state == clientStateRcptCalled {
		act, err := s.DataStart()
		if err != nil || act.Type != ActionContinue {
			return act, err
		}
	}
	if !s.ProtocolOption(OptNoHeaders) && !s.skip {
		for f := hdr.Fields(); f.Next(); {
			act, err := s.HeaderField(f.Key(), f.Value(), nil)
			if err != nil || (act.Type != ActionContinue) {
				return act, err
			}
			if s.skip { // HeaderField() can set s.skip
				break
			}
		}
	}

	return s.HeaderEnd()
}

// BodyChunk sends a single body chunk to the milter.
//
// It is callers responsibility to ensure every chunk is not bigger than
// defined in WithUsedMaxData.
//
// BodyChunk can be called even after the milter responded with ActSkip.
// This method translates a ActSkip milter response into a ActContinue response
// but after a successful ActSkip response Skip will return true.
func (s *ClientSession) BodyChunk(chunk []byte) (*Action, error) {
	if s.state < clientStateHeaderEndCalled || s.state > clientStateBodyChunkCalled {
		return nil, s.errorOut(fmt.Errorf("milter: body: in wrong state %d", s.state))
	}
	s.state = clientStateBodyChunkCalled

	if s.skip {
		return actionContinue, nil
	}

	if s.ProtocolOption(OptNoBody) {
		return actionContinue, nil
	}

	if len(chunk) > int(s.maxBodySize) {
		return nil, s.errorOut(fmt.Errorf("milter: body: too big body chunk: %d > %d", len(chunk), s.maxBodySize))
	}

	if err := s.writePacket(&wire.Message{
		Code: wire.CodeBody,
		Data: chunk,
	}); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: body chunk: %w", err))
	}

	if s.ProtocolOption(OptNoBodyReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(s.ProtocolOption(OptSkip))
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: body chunk: %w", err))
	}
	if act.Type == ActionSkip {
		s.skip = true
		return actionContinue, nil
	}
	return act, nil
}

// BodyReadFrom is a helper function that calls BodyChunk repeatedly to transmit entire
// body from io.Reader and then calls End.
//
// See documentation for these functions for details.
//
// You may first call BodyChunk and then call BodyReadFrom but after BodyReadFrom the End method gets
// called automatically.
func (s *ClientSession) BodyReadFrom(r io.Reader) ([]ModifyAction, *Action, error) {
	if s.state < clientStateHeaderEndCalled || s.state > clientStateBodyChunkCalled {
		return nil, nil, s.errorOut(fmt.Errorf("milter: body: in wrong state %d", s.state))
	}
	if !s.ProtocolOption(OptNoBody) && !s.skip {
		scanner := milterutil.GetFixedBufferScanner(s.maxBodySize, r)
		defer scanner.Close()
		for scanner.Scan() {
			act, err := s.BodyChunk(scanner.Bytes())
			if err != nil {
				return nil, nil, err
			}
			if s.skip { // BodyChunk can set s.skip
				break
			}
			if act.Type != ActionContinue {
				if scanner.Err() != nil {
					return nil, nil, scanner.Err()
				}
				return nil, act, nil
			}
		}
		if scanner.Err() != nil {
			return nil, nil, scanner.Err()
		}
	} else {
		s.state = clientStateBodyChunkCalled
	}

	return s.End()
}

// Skip can be used after a BodyChunk, HeaderField or Rcpt call to check if the milter indicated to not need any more
// of these events. You can directly skip to the next event class. It is not an error to ignore this
// and just keep sending the same events since ClientSession will handle skipping internally.
func (s *ClientSession) Skip() bool {
	return s.skip
}

func (s *ClientSession) readModifyActs() (modifyActs []ModifyAction, act *Action, err error) {
	for {
		msg, err := wire.ReadPacket(s.conn, s.readTimeout)
		if err != nil {
			return nil, nil, fmt.Errorf("action read: %w", err)
		}
		if msg.Code == wire.Code(wire.ActProgress) /* progress */ {
			continue
		}

		switch wire.ModifyActCode(msg.Code) {
		case wire.ActAddRcpt, wire.ActDelRcpt, wire.ActReplBody, wire.ActChangeHeader, wire.ActInsertHeader,
			wire.ActAddHeader, wire.ActChangeFrom, wire.ActQuarantine, wire.ActAddRcptPar:
			modifyAct, err := parseModifyAct(msg)
			if err != nil {
				return nil, nil, err
			}
			modifyActs = append(modifyActs, *modifyAct)
		default:
			act, err = parseAction(msg)
			if err != nil {
				return nil, nil, err
			}

			return modifyActs, act, nil
		}
	}
}

// End sends the EOB message and resets session back to the state before Mail
// call. The same ClientSession can be used to check another message arrived
// within the same SMTP connection (Helo and Conn information is preserved).
//
// Close should be called to conclude session.
func (s *ClientSession) End() ([]ModifyAction, *Action, error) {
	if s.state != clientStateBodyChunkCalled {
		return nil, nil, s.errorOut(fmt.Errorf("milter: end: in wrong state %d", s.state))
	}
	s.state = clientStateHeloCalled
	s.skip = false
	s.skipUnknown = false
	if len(s.macrosByStages) > int(StageEOM) && len(s.macrosByStages[StageEOM]) > 0 {
		if err := s.sendMacros(wire.CodeEOB, s.macrosByStages[StageEOM]); err != nil {
			return nil, nil, s.errorOut(err)
		}
	}
	if err := s.writePacket(&wire.Message{
		Code: wire.CodeEOB,
	}); err != nil {
		return nil, nil, s.errorOut(fmt.Errorf("milter: end: %w", err))
	}

	modifyActs, act, err := s.readModifyActs()
	if err != nil {
		return nil, nil, s.errorOut(fmt.Errorf("milter: end: %w", err))
	}

	return modifyActs, act, nil
}

// Unknown sends an unknown command to the milter. This can happen at any time in the connection.
// Although you should probably do not call it after DataStart until End was called.
//
// You can send macros to the milter with macros. They only get send to the milter when it wants unknown commands.
func (s *ClientSession) Unknown(cmd string, macros map[MacroName]string) (*Action, error) {
	if s.state < clientStateNegotiated || s.state == clientStateError {
		return nil, s.errorOut(fmt.Errorf("milter: unknown: in wrong state %d", s.state))
	}

	if s.ProtocolOption(OptNoUnknown) || s.skipUnknown {
		return actionContinue, nil
	}

	if err := s.sendCmdMacros(wire.CodeUnknown, macros); err != nil {
		return nil, s.errorOut(err)
	}

	msg := &wire.Message{
		Code: wire.CodeUnknown,
	}
	msg.Data = wire.AppendCString(msg.Data, cmd)

	if err := s.writePacket(msg); err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: unknown: %w", err))
	}

	if s.ProtocolOption(OptNoUnknownReply) {
		return actionContinue, nil
	}

	act, err := s.readAction(false)
	if err != nil {
		return nil, s.errorOut(fmt.Errorf("milter: unknown: %w", err))
	}
	return act, nil
}

// Abort sends Abort to the milter. You can call Mail in this same session after a successful call to Abort.
//
// This should be called for a premature but valid end of the SMTP session.
// That is when the SMTP client issues a RSET or QUIT command after at least Helo was called.
//
// You can send macros to the milter with macros. They only get send to the milter when it wants unknown commands.
func (s *ClientSession) Abort(macros map[MacroName]string) error {
	if s.state == clientStateError || s.state < clientStateHeloCalled {
		return s.errorOut(fmt.Errorf("milter: abort: in wrong state %d", s.state))
	}
	s.state = clientStateHeloCalled
	s.skip = false
	s.skipUnknown = false
	if err := s.sendCmdMacros(wire.CodeHeader, macros); err != nil {
		return s.errorOut(err)
	}
	if err := s.writePacket(&wire.Message{
		Code: wire.CodeAbort,
	}); err != nil {
		return s.errorOut(err)
	}

	return nil
}

// Reset sends CodeQuitNewConn to the milter so this session can be used for another connection.
//
// You can use this to do connection pooling - but that could be quite flaky
// since not all milters can handle CodeQuitNewConn
// sendmail or postfix do not use CodeQuitNewConn and never re-use a connection.
// Existing milters might not expect the MTA to use this feature.
func (s *ClientSession) Reset(macros Macros) error {
	if s.state == clientStateError || s.state == clientStateClosed {
		return s.errorOut(fmt.Errorf("milter: reset: in wrong state %d", s.state))
	}
	s.state = clientStateNegotiated
	s.skip = false
	s.skipUnknown = false
	if err := s.writePacket(&wire.Message{
		Code: wire.CodeQuitNewConn,
	}); err != nil {
		return s.errorOut(err)
	}
	s.macros = macros
	return nil
}

// Close releases resources associated with the session and closes the connection to the milter.
//
// If there is a milter sequence in progress the CodeQuit command is called to signal closure to the milter.
//
// You can call Close at any time in the session, and you can call Close multiple times without harm.
func (s *ClientSession) Close() error {
	if s.state == clientStateClosed || s.state == clientStateError {
		return s.closedErr
	}
	s.state = clientStateClosed

	if err := s.writePacket(&wire.Message{
		Code: wire.CodeQuit,
	}); err != nil {
		s.closedErr = fmt.Errorf("milter: close: quit: %w", err)
		_ = s.conn.Close()
		return s.closedErr
	}
	s.closedErr = s.conn.Close()
	return s.closedErr
}
