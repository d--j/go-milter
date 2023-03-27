package milter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

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
	backend     Milter
}

// readPacket reads incoming milter packet
func (m *serverSession) readPacket() (*wire.Message, error) {
	return wire.ReadPacket(m.conn, 0)
}

// writePacket sends a milter response packet to socket stream
func (m *serverSession) writePacket(msg *wire.Message) error {
	return wire.WritePacket(m.conn, msg, 0)
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
	} else {
		if mtaVersion < 2 || mtaVersion > MaxServerProtocolVersion {
			return nil, fmt.Errorf("milter: negotiate: unsupported protocol version: %d", mtaVersion)
		}
		m.version = mtaVersion
		if milterActions&mtaActionMask != milterActions {
			return nil, fmt.Errorf("milter: negotiate: MTA does not offer required actions. offered: %032b requested: %032b", mtaActionMask, milterActions)
		}
		m.actions = milterActions & mtaActionMask
		if milterProtocol&mtaProtoMask != milterProtocol {
			return nil, fmt.Errorf("milter: negotiate: MTA does not offer required protocol options. offered: %032b requested: %032b", mtaProtoMask, milterProtocol)
		}
		m.protocol = milterProtocol & mtaProtoMask
		maxDataSize = offeredMaxDataSize
	}
	if m.version < 2 || m.version > MaxServerProtocolVersion {
		return nil, fmt.Errorf("milter: negotiate: unsupported protocol version: %d", m.version)
	}
	if maxDataSize != DataSize64K && maxDataSize != DataSize256K && maxDataSize != DataSize1M {
		maxDataSize = DataSize64K
	}
	if usedMaxData == 0 {
		usedMaxData = maxDataSize
	}
	m.maxDataSize = usedMaxData

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

func (m *serverSession) newBackend() Milter {
	return m.server.options.newMilter(m.version, m.actions, m.protocol, m.maxDataSize)
}

// Process processes incoming milter commands
func (m *serverSession) Process(msg *wire.Message) (*Response, error) {
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
			// also accept [dead::cafe] style IPv6 addresses
			if len(address) > 2 && address[0] == '[' && address[len(address)-1] == ']' {
				addr = net.ParseIP(address[1 : len(address)-1])
				if addr != nil {
					address = addr.String()
				}
			} else if strings.HasPrefix(address, "IPv6:") {
				addr = net.ParseIP(address[5:])
			} else {
				addr = net.ParseIP(address)
			}
			if addr == nil {
				return nil, fmt.Errorf("milter: conn: unexpected ip6 address: %q", address)
			}
		default:
			return nil, fmt.Errorf("milter: conn: unexpected protocol family: %c", protocolFamily)
		}
		// run handler and return
		return m.backend.Connect(
			hostname,
			family,
			port,
			address,
			newModifier(m, true))

	case wire.CodeHelo:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: helo: unexpected data size: %d", len(msg.Data))
		}
		m.macros.DelStageAndAbove(StageMail)
		name := wire.ReadCString(msg.Data)
		return m.backend.Helo(name, newModifier(m, true))

	case wire.CodeMail:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: mail: unexpected data size: %d", len(msg.Data))
		}
		m.macros.DelStageAndAbove(StageRcpt)
		from := wire.ReadCString(msg.Data)
		msg.Data = msg.Data[len(from)+1:]
		esmtpArgs := wire.ReadCString(msg.Data)
		return m.backend.MailFrom(RemoveAngle(from), esmtpArgs, newModifier(m, true))

	case wire.CodeRcpt:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: rcpt: unexpected data size: %d", len(msg.Data))
		}
		m.macros.DelStageAndAbove(StageData)
		to := wire.ReadCString(msg.Data)
		msg.Data = msg.Data[len(to)+1:]
		esmtpArgs := wire.ReadCString(msg.Data)
		return m.backend.RcptTo(RemoveAngle(to), esmtpArgs, newModifier(m, true))

	case wire.CodeData:
		m.macros.DelStageAndAbove(StageEOH)
		return m.backend.Data(newModifier(m, true))

	case wire.CodeHeader:
		if len(msg.Data) < 2 {
			return nil, fmt.Errorf("milter: header: unexpected data size: %d", len(msg.Data))
		}
		// add new header to headers map
		headerData := wire.DecodeCStrings(msg.Data)
		if len(headerData) != 2 {
			return nil, fmt.Errorf("milter: header: unexpected number of strings: %d", len(headerData))
		}
		// call and return milter handler
		resp, err := m.backend.Header(headerData[0], headerData[1], newModifier(m, true))
		m.macros.DelStageAndAbove(StageEndMarker)
		return resp, err

	case wire.CodeEOH:
		m.macros.DelStageAndAbove(StageEOM)
		return m.backend.Headers(newModifier(m, true))

	case wire.CodeBody:
		resp, err := m.backend.BodyChunk(msg.Data, newModifier(m, true))
		m.macros.DelStageAndAbove(StageEndMarker)
		return resp, err

	case wire.CodeEOB:
		return m.backend.EndOfMessage(newModifier(m, false))

	case wire.CodeUnknown:
		cmd := wire.ReadCString(msg.Data)
		resp, err := m.backend.Unknown(cmd, newModifier(m, true))
		m.macros.DelStageAndAbove(StageEndMarker)
		return resp, err

	case wire.CodeMacro:
		if len(msg.Data) == 0 {
			return nil, fmt.Errorf("milter: macro: unexpected data size: %d", len(msg.Data))
		}
		code := wire.Code(msg.Data[0])
		var stage MacroStage
		switch code {
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
			LogWarning("MTA sent macro for %c. we cannot handle this so we ignore it", code)
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
		err := m.backend.Abort(newModifier(m, true))
		m.macros.DelStageAndAbove(StageHelo)
		return nil, err

	case wire.CodeQuitNewConn:
		// abort current connection and start over
		m.backend.Cleanup()
		m.macros.DelStageAndAbove(StageConnect)
		m.backend = m.newBackend()
		// do not send response
		return nil, nil

	case wire.CodeQuit:
		m.backend.Cleanup()
		// client requested session close
		return nil, errCloseSession

	default:
		// print error and close session
		LogWarning("Unrecognized command code: %c", msg.Code)
		return nil, errCloseSession
	}
}

// HandleMilterCommands processes all milter commands in the same connection
func (m *serverSession) HandleMilterCommands() {
	defer func() {
		if m.backend != nil {
			m.backend.Cleanup()
		}
		if m.conn != nil {
			if err := m.conn.Close(); err != nil && err != io.EOF {
				LogWarning("Error closing connection: %v", err)
			}
		}
	}()

	// first do the negotiation
	msg, err := m.readPacket()
	if err != nil {
		if err != io.EOF {
			LogWarning("Error reading milter command: %v", err)
		}
		return
	}
	resp, err := m.negotiate(msg, m.server.options.maxVersion, m.server.options.actions, m.server.options.protocol, m.server.options.negotiationCallback, m.server.options.macrosByStage, 0)
	if err != nil {
		LogWarning("Error negotiating: %v", err)
		return
	}
	m.backend = m.newBackend()
	if err = m.writePacket(resp.Response()); err != nil {
		LogWarning("Error writing packet: %v", err)
		return
	}

	// now we can process the events
	for {
		msg, err := m.readPacket()
		if err != nil {
			if err != io.EOF {
				LogWarning("Error reading milter command: %v", err)
			}
			return
		}

		resp, err := m.Process(msg)
		if err != nil {
			if err != errCloseSession {
				// log error condition
				LogWarning("Error performing milter command: %v", err)
				if resp != nil && !m.skipResponse(msg.Code) {
					_ = m.writePacket(resp.Response())
				}
			}
			return
		}

		// ignore empty responses or responses we indicated to not send
		if resp == nil || m.skipResponse(msg.Code) {
			continue
		}

		// send back response message
		if err = m.writePacket(resp.Response()); err != nil {
			LogWarning("Error writing packet: %v", err)
			return
		}

		if !resp.Continue() {
			m.backend.Cleanup()
			// prepare backend for next message
			m.backend = m.newBackend()
			m.macros.DelStageAndAbove(StageMail)
		}
	}
}

// protocolOption checks whether the option is set in negotiated options, that
// is, requested by the milter and offered by the MTA.
func (m *serverSession) protocolOption(opt OptProtocol) bool {
	return m.protocol&opt != 0
}

func (m *serverSession) skipResponse(code wire.Code) bool {
	switch code {
	case wire.CodeConn:
		return m.protocolOption(OptNoConnReply)
	case wire.CodeHelo:
		return m.protocolOption(OptNoHeloReply)
	case wire.CodeMail:
		return m.protocolOption(OptNoMailReply)
	case wire.CodeRcpt:
		return m.protocolOption(OptNoRcptReply)
	case wire.CodeData:
		return m.protocolOption(OptNoDataReply)
	case wire.CodeUnknown:
		return m.protocolOption(OptNoUnknownReply)
	case wire.CodeEOH:
		return m.protocolOption(OptNoEOHReply)
	case wire.CodeHeader:
		return m.protocolOption(OptNoHeaderReply)
	case wire.CodeBody:
		return m.protocolOption(OptNoBodyReply)
	default:
		return false
	}
}
