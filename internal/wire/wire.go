// Package wire includes constants and functions for the raw libmilter protocol
package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

//go:generate go tool stringer -type=Code,ActionCode,ModifyActCode -output=wire_string.go

type Code byte

// Message represents a command sent from milter client
type Message struct {
	Code Code
	Data []byte
}

// MacroCode returns the Code this CodeMacro message is about.
// If the message is not a CodeMacro message, it returns the Code itself.
func (m *Message) MacroCode() Code {
	if m.Code == CodeMacro && len(m.Data) > 0 {
		return Code(m.Data[0])
	}
	return m.Code
}

type ActionCode byte

const (
	ActAccept    ActionCode = 'a' // SMFIR_ACCEPT
	ActContinue  ActionCode = 'c' // SMFIR_CONTINUE
	ActDiscard   ActionCode = 'd' // SMFIR_DISCARD
	ActReject    ActionCode = 'r' // SMFIR_REJECT
	ActTempFail  ActionCode = 't' // SMFIR_TEMPFAIL
	ActReplyCode ActionCode = 'y' // SMFIR_REPLYCODE
	ActSkip      ActionCode = 's' // SMFIR_SKIP [v6]
	ActProgress  ActionCode = 'p' // SMFIR_PROGRESS [v6]
)

type ModifyActCode byte

const (
	ActAddRcpt      ModifyActCode = '+' // SMFIR_ADDRCPT
	ActDelRcpt      ModifyActCode = '-' // SMFIR_DELRCPT
	ActReplBody     ModifyActCode = 'b' // SMFIR_ACCEPT
	ActAddHeader    ModifyActCode = 'h' // SMFIR_ADDHEADER
	ActChangeHeader ModifyActCode = 'm' // SMFIR_CHGHEADER
	ActInsertHeader ModifyActCode = 'i' // SMFIR_INSHEADER
	ActQuarantine   ModifyActCode = 'q' // SMFIR_QUARANTINE
	ActChangeFrom   ModifyActCode = 'e' // SMFIR_CHGFROM [v6]
	ActAddRcptPar   ModifyActCode = '2' // SMFIR_ADDRCPT_PAR [v6]
)

const (
	CodeOptNeg      Code = 'O' // SMFIC_OPTNEG
	CodeMacro       Code = 'D' // SMFIC_MACRO
	CodeConn        Code = 'C' // SMFIC_CONNECT
	CodeQuit        Code = 'Q' // SMFIC_QUIT
	CodeHelo        Code = 'H' // SMFIC_HELO
	CodeMail        Code = 'M' // SMFIC_MAIL
	CodeRcpt        Code = 'R' // SMFIC_RCPT
	CodeHeader      Code = 'L' // SMFIC_HEADER
	CodeEOH         Code = 'N' // SMFIC_EOH
	CodeBody        Code = 'B' // SMFIC_BODY
	CodeEOB         Code = 'E' // SMFIC_BODYEOB
	CodeAbort       Code = 'A' // SMFIC_ABORT
	CodeData        Code = 'T' // SMFIC_DATA
	CodeQuitNewConn Code = 'K' // SMFIC_QUIT_NC [v6]
	CodeUnknown     Code = 'U' // SMFIC_UNKNOWN [v6]
)

// We reject reading/writing messages larger than 512 MB outright.
const maxPacketSize = 512 * 1024 * 1024

func ReadPacket(conn net.Conn, timeout time.Duration) (*Message, error) {
	if timeout != 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		defer func(conn net.Conn) {
			_ = conn.SetReadDeadline(time.Time{})
		}(conn)
	}

	// read packet length
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	if length > maxPacketSize {
		return nil, fmt.Errorf("milter: reject to read %d bytes in one message", length)
	}

	// read packet data
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}

	// prepare response data
	message := Message{
		Code: Code(data[0]),
		Data: data[1:],
	}

	return &message, nil
}

func WritePacket(conn net.Conn, msg *Message, timeout time.Duration) error {
	if msg == nil {
		return errors.New("msg nil pointer")
	}
	if timeout != 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(timeout))
		defer func(conn net.Conn) {
			_ = conn.SetWriteDeadline(time.Time{})
		}(conn)
	}

	length := len(msg.Data) + 1
	if length > maxPacketSize {
		return fmt.Errorf("milter: cannot write %d bytes in one message", length)
	}

	_, err := conn.Write([]byte{byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length), byte(msg.Code)})
	if err != nil {
		return err
	}

	if len(msg.Data) == 0 {
		return nil
	}
	_, err = conn.Write(msg.Data)

	return err
}

// AppendUint16 appends the big endian encoding of val to dest. It returns the new dest like append does.
func AppendUint16(dest []byte, val uint16) []byte {
	return append(dest, byte(val>>8), byte(val))
}
