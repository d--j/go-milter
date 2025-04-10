package main

import (
	"fmt"
	"log"

	"github.com/d--j/go-milter"
)

type LogMilter struct {
	id          uint64
	macroValues map[milter.MacroName]string
}

func (l *LogMilter) log(format string, v ...any) {
	log.Printf(fmt.Sprintf("[%d] %s", l.id, format), v...)
}

func (l *LogMilter) NewConnection(m milter.Modifier) error {
	l.id = m.MilterId()
	l.log("NewConnection")
	return nil
}

func (l *LogMilter) Connect(host string, family string, port uint16, addr string, m milter.Modifier) (*milter.Response, error) {
	l.log("CONNECT host = %q, family = %q, port = %d, addr = %q", host, family, port, addr)
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) Helo(name string, m milter.Modifier) (*milter.Response, error) {
	l.log("HELO %q", name)
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) MailFrom(from string, esmtpArgs string, m milter.Modifier) (*milter.Response, error) {
	l.log("MAIL FROM <%s> %s", from, esmtpArgs)
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) RcptTo(rcptTo string, esmtpArgs string, m milter.Modifier) (*milter.Response, error) {
	l.log("RCPT TO <%s> %s", rcptTo, esmtpArgs)
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) Data(m milter.Modifier) (*milter.Response, error) {
	l.log("DATA")
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) Header(name string, value string, m milter.Modifier) (*milter.Response, error) {
	l.log("HEADER %s: %q", name, value)
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) Headers(m milter.Modifier) (*milter.Response, error) {
	l.log("EOH")
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) BodyChunk(chunk []byte, m milter.Modifier) (*milter.Response, error) {
	l.log("BODY CHUNK size = %d", len(chunk))
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) EndOfMessage(m milter.Modifier) (*milter.Response, error) {
	l.log("EOM")
	l.outputChangedMacros(m)
	return milter.RespAccept, nil
}

func (l *LogMilter) Abort(m milter.Modifier) error {
	l.log("ABORT")
	l.outputChangedMacros(m)
	return nil
}

func (l *LogMilter) Unknown(cmd string, m milter.Modifier) (*milter.Response, error) {
	l.log("UNKNOWN %q", cmd)
	l.outputChangedMacros(m)
	return milter.RespContinue, nil
}

func (l *LogMilter) Cleanup(m milter.Modifier) {
	l.log("cleanup")
	l.macroValues = nil
}

func (l *LogMilter) outputChangedMacros(m milter.Modifier) {
	if l.macroValues == nil {
		l.macroValues = make(map[milter.MacroName]string)
	}
	for _, name := range []milter.MacroName{
		milter.MacroMTAVersion,
		milter.MacroMTAFQDN,
		milter.MacroDaemonName,
		milter.MacroDaemonAddr,
		milter.MacroDaemonPort,
		milter.MacroIfName,
		milter.MacroIfAddr,
		milter.MacroTlsVersion,
		milter.MacroCipher,
		milter.MacroCipherBits,
		milter.MacroCertSubject,
		milter.MacroCertIssuer,
		milter.MacroClientAddr,
		milter.MacroClientPort,
		milter.MacroClientName,
		milter.MacroClientPTR,
		milter.MacroClientConnections,
		milter.MacroQueueId,
		milter.MacroAuthType,
		milter.MacroAuthAuthen,
		milter.MacroAuthSsf,
		milter.MacroAuthAuthor,
		milter.MacroMailMailer,
		milter.MacroMailHost,
		milter.MacroMailAddr,
		milter.MacroRcptMailer,
		milter.MacroRcptHost,
		milter.MacroRcptAddr,
		milter.MacroRFC1413AuthInfo,
		milter.MacroHopCount,
		milter.MacroSenderHostName,
		milter.MacroProtocolUsed,
		milter.MacroMTAPid,
		milter.MacroDateRFC822Origin,
		milter.MacroDateRFC822Current,
		milter.MacroDateANSICCurrent,
		milter.MacroDateSecondsCurrent,
	} {
		oldValue := l.macroValues[name]
		newValue := m.Get(name)
		if oldValue != newValue {
			if oldValue != "" {
				l.log("  macro %s value %q -> %q", name, oldValue, newValue)
			} else {
				l.log("  macro %s value %q", name, newValue)
			}
		}
		l.macroValues[name] = newValue
	}
}

var _ milter.Milter = (*LogMilter)(nil)
