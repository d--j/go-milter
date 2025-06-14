package milter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/d--j/go-milter/internal/wire"
)

var errCloseSession = errors.New("stop current milter processing")

// serverSession keeps session state during MTA communication
type serverSession struct {
	server      *Server
	version     uint32
	actions     OptAction
	protocol    OptProtocol
	maxDataSize DataSize
	conn        net.Conn
	macros      *macrosStages
	backendId   uint64
	mu          sync.Mutex
	modifier    *modifier
}

// init sets up the internal state of the session
func (m *serverSession) init(server *Server, conn net.Conn, version uint32, actions OptAction, protocol OptProtocol) {
	m.server = server
	m.conn = conn
	m.version = version
	m.actions = actions
	m.protocol = protocol
	m.macros = newMacroStages()
}

// readPacket reads incoming milter packet
func (m *serverSession) readPacket(timeout time.Duration) (*wire.Message, error) {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return nil, errCloseSession
	}
	return wire.ReadPacket(conn, timeout)
}

// writePacket sends a milter response packet to socket stream
func (m *serverSession) writePacket(msg *wire.Message) error {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return errCloseSession
	}
	return wire.WritePacket(conn, msg, m.server.options.writeTimeout)
}

func (m *serverSession) negotiate(msg *wire.Message, milterVersion uint32, milterActions OptAction, milterProtocol OptProtocol, callback NegotiationCallbackFunc, macroRequests macroRequests, usedMaxData DataSize) (*Response, error) {
	if msg.Code != wire.CodeOptNeg {
		return nil, fmt.Errorf("milter: negotiate: unexpected package with code %c", msg.Code)
	}
	if len(msg.Data) < 4*3 /* version + action mask + proto mask */ {
		return nil, fmt.Errorf("milter: negotiate: unexpected data size: %d", len(msg.Data))
	}
	mtaVersion := binary.BigEndian.Uint32(msg.Data[:4])
	mtaActionMask := OptAction(binary.BigEndian.Uint32(msg.Data[4:]))
	mtaProtoMask := OptProtocol(binary.BigEndian.Uint32(msg.Data[8:]))
	offeredMaxDataSize := DataSize64K
	if uint32(mtaProtoMask)&optMds1M == optMds1M {
		offeredMaxDataSize = DataSize1M
	} else if uint32(mtaProtoMask)&optMds256K == optMds256K {
		offeredMaxDataSize = DataSize256K
	}
	mtaProtoMask = mtaProtoMask & (^OptProtocol(optInternal))

	var err error
	var maxDataSize DataSize
	if callback != nil {
		if m.version, m.actions, m.protocol, maxDataSize, err = callback(mtaVersion, milterVersion, mtaActionMask, milterActions, mtaProtoMask, milterProtocol, offeredMaxDataSize); err != nil {
			return nil, err
		}
		if m.version < 2 || m.version > MaxServerProtocolVersion {
			return nil, fmt.Errorf("milter: negotiate: unsupported protocol version: %d", m.version)
		}
	} else {
		if mtaVersion < 2 || mtaVersion > MaxServerProtocolVersion {
			return nil, fmt.Errorf("milter: negotiate: unsupported protocol version: %d", mtaVersion)
		}
		m.version = mtaVersion
		if milterActions&mtaActionMask != milterActions {
			return nil, fmt.Errorf("milter: negotiate: MTA does not offer required actions. offered: %q requested: %q", mtaActionMask, milterActions)
		}
		m.actions = milterActions & mtaActionMask
		if milterProtocol&mtaProtoMask != milterProtocol {
			return nil, fmt.Errorf("milter: negotiate: MTA does not offer required protocol options. offered: %q requested: %q", mtaProtoMask, milterProtocol)
		}
		m.protocol = milterProtocol & mtaProtoMask
		maxDataSize = offeredMaxDataSize
	}
	if maxDataSize != DataSize64K && maxDataSize != DataSize256K && maxDataSize != DataSize1M {
		maxDataSize = DataSize64K
	}
	if usedMaxData == 0 {
		usedMaxData = maxDataSize
	}
	m.maxDataSize = usedMaxData
	m.modifier = newModifier(m, modifierStateReadOnly)

	// TODO: activate skip response according to m.version

	sizeMask := uint32(0)
	if maxDataSize == DataSize256K {
		sizeMask = optMds256K
	} else if maxDataSize == DataSize1M {
		sizeMask = optMds1M
	}

	// prepare response data
	var buffer bytes.Buffer
	for _, value := range []uint32{m.version, uint32(m.actions), uint32(m.protocol) | sizeMask} {
		if err := binary.Write(&buffer, binary.BigEndian, value); err != nil {
			return nil, fmt.Errorf("milter: negotiate: %w", err)
		}
	}
	// send the macros we want to have in the response
	if macroRequests != nil && mtaActionMask&OptSetMacros != 0 {
		for st := 0; st < int(StageEndMarker) && st < len(macroRequests); st++ {
			if macroRequests[st] != nil && len(macroRequests[st]) > 0 {
				if err := binary.Write(&buffer, binary.BigEndian, uint32(st)); err != nil {
					return nil, fmt.Errorf("milter: negotiate: %w", err)
				}
				buffer.WriteString(strings.Join(macroRequests[st], " "))
				buffer.WriteByte(0)
			}
		}
	} else if macroRequests != nil {
		LogWarning("milter could not send the needed macros since MTA does not support this")
	}
	// build negotiation response
	return newResponse(wire.CodeOptNeg, buffer.Bytes()), nil
}

// processMsg processes incoming milter commands
func (m *serverSession) processMsg(backend Milter, msg *wire.Message) (*Response, error) {
	switch msg.Code {
	case wire.CodeOptNeg:
		return nil, fmt.Errorf("milter: negotiate: can only be called once in a connection")

	case wire.CodeConn:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: conn: unexpected data size: %d", len(msg.Data))
		}
		m.macros.DelStageAndAbove(StageHelo)
		hostname := wire.ReadCString(msg.Data)
		msg.Data = msg.Data[len(hostname)+1:]
		// get protocol family
		protocolFamily := msg.Data[0]
		msg.Data = msg.Data[1:]
		// get port and address
		var port uint16
		var address string
		if protocolFamily == 'L' || protocolFamily == '4' || protocolFamily == '6' {
			if len(msg.Data) < 2 {
				return nil, fmt.Errorf("milter: conn: unexpected data size: %d", len(msg.Data))
			}
			port = binary.BigEndian.Uint16(msg.Data)
			msg.Data = msg.Data[2:]
			// get address
			address = wire.ReadCString(msg.Data)
		}
		// convert family to human-readable string and validate
		family := ""
		switch protocolFamily {
		case 'U':
			family = "unknown"
		case 'L':
			family = "unix"
		case '4':
			family = "tcp4"
			addr := net.ParseIP(address)
			if addr == nil || addr.To4() == nil {
				return nil, fmt.Errorf("milter: conn: unexpected ip4 address: %q", address)
			}
		case '6':
			family = "tcp6"
			var addr net.IP
			// remove optional IPv6: prefix
			address = strings.TrimPrefix(address, "IPv6:")
			// also accept [dead::cafe] style IPv6 addresses
			if len(address) > 2 && address[0] == '[' && address[len(address)-1] == ']' {
				addr = net.ParseIP(address[1 : len(address)-1])
			} else {
				addr = net.ParseIP(address)
			}
			if addr == nil {
				return nil, fmt.Errorf("milter: conn: unexpected ip6 address: %q", address)
			}
			address = addr.String()
		default:
			return nil, fmt.Errorf("milter: conn: unexpected protocol family: %c", protocolFamily)
		}
		// run handler and return
		resp, err := backend.Connect(hostname, family, port, address, m.modifier.withState(modifierStateProgressOnly))
		return resp, err

	case wire.CodeHelo:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: helo: unexpected data size: %d", len(msg.Data))
		}
		m.macros.DelStageAndAbove(StageMail)
		name := wire.ReadCString(msg.Data)
		resp, err := backend.Helo(name, m.modifier.withState(modifierStateProgressOnly))
		return resp, err

	case wire.CodeMail:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: mail: unexpected data size: %d", len(msg.Data))
		}
		m.macros.DelStageAndAbove(StageRcpt)
		from := wire.ReadCString(msg.Data)
		data := msg.Data[len(from)+1:]

		// the rest of the data are ESMTP arguments, separated by a zero byte.
		esmtpArgs := strings.Join(wire.DecodeCStrings(data), " ")
		resp, err := backend.MailFrom(RemoveAngle(from), esmtpArgs, m.modifier.withState(modifierStateProgressOnly))
		return resp, err

	case wire.CodeRcpt:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: rcpt: unexpected data size: %d", len(msg.Data))
		}
		m.macros.DelStageAndAbove(StageData)
		to := wire.ReadCString(msg.Data)
		rest := msg.Data[len(to)+1:]

		// the rest of the data are ESMTP arguments, separated by a zero byte.
		esmtpArgs := strings.Join(wire.DecodeCStrings(rest), " ")
		resp, err := backend.RcptTo(RemoveAngle(to), esmtpArgs, m.modifier.withState(modifierStateProgressOnly))
		return resp, err

	case wire.CodeData:
		m.macros.DelStageAndAbove(StageEOH)
		resp, err := backend.Data(m.modifier.withState(modifierStateProgressOnly))
		return resp, err

	case wire.CodeHeader:
		if len(msg.Data) < 2 {
			return nil, fmt.Errorf("milter: header: unexpected data size: %d", len(msg.Data))
		}
		headerData := wire.DecodeCStrings(msg.Data)
		if len(headerData) != 2 {
			return nil, fmt.Errorf("milter: header: unexpected number of strings: %d", len(headerData))
		}
		resp, err := backend.Header(headerData[0], headerData[1], m.modifier.withState(modifierStateProgressOnly))
		m.macros.DelStageAndAbove(StageEndMarker)
		return resp, err

	case wire.CodeEOH:
		m.macros.DelStageAndAbove(StageEOM)
		resp, err := backend.Headers(m.modifier.withState(modifierStateProgressOnly))
		return resp, err

	case wire.CodeBody:
		resp, err := backend.BodyChunk(msg.Data, m.modifier.withState(modifierStateProgressOnly))
		m.macros.DelStageAndAbove(StageEndMarker)
		return resp, err

	case wire.CodeEOB:
		resp, err := backend.EndOfMessage(m.modifier.withState(modifierStateReadWrite))
		if err == nil && (resp == nil || resp.Continue()) {
			// if the backend does not return a response or one that does not terminate, we assume it is a success
			resp = RespAccept
		}
		return resp, err

	case wire.CodeUnknown:
		cmd := wire.ReadCString(msg.Data)
		resp, err := backend.Unknown(cmd, m.modifier.withState(modifierStateProgressOnly))
		m.macros.DelStageAndAbove(StageEndMarker)
		return resp, err

	case wire.CodeMacro:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: macro: unexpected data size: %d", len(msg.Data))
		}
		var stage MacroStage
		switch msg.MacroCode() {
		case wire.CodeConn:
			stage = StageConnect
		case wire.CodeHelo:
			stage = StageHelo
		case wire.CodeMail:
			stage = StageMail
		case wire.CodeRcpt:
			stage = StageRcpt
		case wire.CodeData:
			stage = StageData
		case wire.CodeEOH:
			stage = StageEOH
		case wire.CodeEOB:
			stage = StageEOM
		case wire.CodeUnknown, wire.CodeHeader, wire.CodeAbort, wire.CodeBody:
			stage = StageEndMarker // this stage gets cleared after the command
		default:
			LogWarning("MTA sent macro for %c. we cannot handle this so we ignore it", msg.MacroCode())
			return nil, nil
		}
		m.macros.DelStageAndAbove(stage)
		// convert data to Go strings
		data := wire.DecodeCStrings(msg.Data[1:])
		if len(data) != 0 {
			if len(data)%2 == 1 {
				data = append(data, "")
			}
			m.macros.SetStage(stage, data...)
		}
		// do not send response
		return nil, nil

	case wire.CodeAbort:
		// abort current message and start over
		err := backend.Abort(m.modifier.withState(modifierStateReadOnly))
		m.macros.DelStageAndAbove(StageHelo)
		return nil, err

	case wire.CodeQuitNewConn:
		// abort current connection and start over
		m.macros.DelStageAndAbove(StageConnect)
		return nil, backend.NewConnection(m.modifier.withState(modifierStateReadOnly))

	case wire.CodeQuit:
		// client requested session close, we handle the session end in HandleMilterCommands
		return nil, nil

	default:
		// print error and close session
		LogWarning("Unrecognized command code: %s", msg.Code)
		return nil, errCloseSession
	}
}

// ignoreError checks if the error is a closing error
// It checks for EOF, net.ErrClosed, and errCloseSession
func ignoreError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, errCloseSession) || errors.Is(err, net.ErrClosed)
}

func (m *serverSession) closeConn() {
	m.mu.Lock()
	conn := m.conn
	m.conn = nil
	m.mu.Unlock()
	if conn != nil {
		if err := conn.Close(); err != nil && !ignoreError(err) {
			LogWarning("Error closing connection: %v", err)
		}
	}
}

// HandleMilterCommands processes all milter commands in the same connection
func (m *serverSession) HandleMilterCommands() {
	defer m.closeConn()

	// first do the negotiation - with a hard-coded timeout of 1 second
	msg, err := m.readPacket(time.Second)
	if err != nil {
		if !ignoreError(err) {
			LogWarning("Error reading milter command: %v", err)
		}
		return
	}
	resp, err := m.negotiate(msg, m.server.options.maxVersion, m.server.options.actions, m.server.options.protocol, m.server.options.negotiationCallback, m.server.options.macrosByStage, 0)
	if err != nil {
		if !ignoreError(err) {
			LogWarning("Error negotiating: %v", err)
		}
		return
	}
	if err = m.writePacket(resp.Response()); err != nil {
		if !ignoreError(err) {
			LogWarning("Error writing packet: %v", err)
		}
		return
	}
	var backend Milter
	backend, m.backendId = m.server.newMilter(m.version, m.actions, m.protocol, m.maxDataSize)
	m.modifier.milterId = m.backendId
	defer func() {
		backend.Cleanup(m.modifier.withState(modifierStateReadOnly))
	}()
	if err := backend.NewConnection(m.modifier.withState(modifierStateReadOnly)); err != nil {
		return
	}

	lastCode := wire.CodeOptNeg
	lastOrder := 0
	codeOrderMap := map[wire.Code]int{
		wire.CodeConn:   1,
		wire.CodeHelo:   2,
		wire.CodeMail:   3,
		wire.CodeRcpt:   4,
		wire.CodeData:   5,
		wire.CodeHeader: 6,
		wire.CodeEOH:    7,
		wire.CodeBody:   8,
		wire.CodeEOB:    9,
	}
	readTimeout := m.server.options.readTimeout
	hasDecision := false

	// now we can process the events
	for {
		msg, err = m.readPacket(readTimeout)
		if err != nil {
			if !ignoreError(err) {
				LogWarning("Error reading milter command: %v", err)
			}
			return
		}

		// Postfix always sends us an Abort when an SMTP connection gets reused.
		// Sendmail does not do that when we accepted/rejected the message before EOB.
		// We synthesize an Abort message to the backend when we detect that an Abort was not sent.
		// This is not really necessary (the backend should be able to handle this), but it does not hurt
		// and makes Milter backend development less error-prone.
		code := msg.MacroCode()
		currentCommand, ok := codeOrderMap[code]
		if ok {
			if lastOrder > currentCommand && lastCode != wire.CodeAbort {
				_, err = m.processMsg(backend, &wire.Message{Code: wire.CodeAbort})
				if err != nil {
					if !ignoreError(err) {
						// log error condition
						LogWarning("Error performing milter command: %v", err)
						if resp != nil && !m.skipResponse(msg.Code) {
							_ = m.writePacket(resp.Response())
						}
					}
					return
				}
			}
			lastOrder = currentCommand
		} else {
			// Postfix sometimes sends us multiple Aborts - one is totally enough, so filter them out
			if code == wire.CodeAbort && lastCode == wire.CodeAbort {
				continue
			}
		}
		lastCode = code

		var resp *Response
		resp, err = m.processMsg(backend, msg)
		if err != nil {
			if !ignoreError(err) {
				// log error condition
				LogWarning("Error performing milter command: %v", err)
				if resp != nil && !m.skipResponse(msg.Code) {
					_ = m.writePacket(resp.Response())
				}
			}
			return
		}
		hasDecision = resp != nil && !resp.Continue()
		if msg.Code == wire.CodeRcpt && hasDecision && resp != RespDiscard {
			hasDecision = false
		}
		if hasDecision {
			m.macros.DelStageAndAbove(StageMail)
		}

		// if we have a response and did not tell the MTA to skip the response
		if resp != nil && !m.skipResponse(msg.Code) {
			// send back response message
			if err = m.writePacket(resp.Response()); err != nil {
				if !ignoreError(err) {
					LogWarning("Error writing packet: %v", err)
				}
				return
			}
		}

		if msg.Code == wire.CodeQuit {
			return
		}

		// Gracefully exit only after MTA send CodeQuitNewConn (CodeQuit always exits anyway)
		// We cannot exit at other commands since this would break the milter connection mid-way in an SMTP connection.
		// The MTA would temp fail (depends on configuration) all commands of this SMTP connection after we broke the
		// milter connection.
		if msg.Code == wire.CodeQuitNewConn && m.server.shuttingDown() {
			return
		}
	}
}

func (m *serverSession) skipResponse(code wire.Code) bool {
	switch code {
	case wire.CodeConn:
		return m.protocol&OptNoConnReply != 0
	case wire.CodeHelo:
		return m.protocol&OptNoHeloReply != 0
	case wire.CodeMail:
		return m.protocol&OptNoMailReply != 0
	case wire.CodeRcpt:
		return m.protocol&OptNoRcptReply != 0
	case wire.CodeData:
		return m.protocol&OptNoDataReply != 0
	case wire.CodeUnknown:
		return m.protocol&OptNoUnknownReply != 0
	case wire.CodeEOH:
		return m.protocol&OptNoEOHReply != 0
	case wire.CodeHeader:
		return m.protocol&OptNoHeaderReply != 0
	case wire.CodeBody:
		return m.protocol&OptNoBodyReply != 0
	default:
		return false
	}
}
