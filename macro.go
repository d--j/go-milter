package milter

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

type MacroStage = byte

const (
	StageConnect        MacroStage = iota // SMFIM_CONNECT
	StageHelo                             // SMFIM_HELO
	StageMail                             // SMFIM_ENVFROM
	StageRcpt                             // SMFIM_ENVRCPT
	StageData                             // SMFIM_DATA
	StageEOM                              // SMFIM_EOM
	StageEOH                              // SMFIM_EOH
	StageEndMarker                        // is used for command level macros for Abort, Unknown and Header commands
	stageNotFoundMarker                   // identifies that a macro was not found. Always needs to be StageEndMarker + 1.
)

type MacroName = string

// Macros that have good support between MTAs like sendmail and Postfix
const (
	MacroMTAVersion        MacroName = "v"                    // MTA Version (and MTA name in case of Postfix)
	MacroMTAFQDN           MacroName = "j"                    // MTA fully qualified domain name
	MacroDaemonName        MacroName = "{daemon_name}"        // name of the daemon of the MTA. E.g. MTA-v4 or smtpd or anything the user configured.
	MacroDaemonAddr        MacroName = "{daemon_addr}"        // Local server IP address
	MacroDaemonPort        MacroName = "{daemon_port}"        // Local server TCP port
	MacroIfName            MacroName = "{if_name}"            // Interface name of the interface the MTA is accepting the SMTP connection
	MacroIfAddr            MacroName = "{if_addr}"            // IP address of the interface the MTA is accepting the SMTP connection
	MacroTlsVersion        MacroName = "{tls_version}"        // TLS version in use (set after STARTTLS or when SMTPS is used)
	MacroCipher            MacroName = "{cipher}"             // Cipher suite used (set after STARTTLS or when SMTPS is used)
	MacroCipherBits        MacroName = "{cipher_bits}"        // Strength of the cipher suite in bits (set after STARTTLS or when SMTPS is used)
	MacroCertSubject       MacroName = "{cert_subject}"       // Validated client cert's subject information (only when mutual TLS is in use)
	MacroCertIssuer        MacroName = "{cert_issuer}"        // Validated client cert's issuer information (only when mutual TLS is in use)
	MacroClientAddr        MacroName = "{client_addr}"        // Remote client IP address
	MacroClientPort        MacroName = "{client_port}"        // Remote client TCP port
	MacroClientPTR         MacroName = "{client_ptr}"         // Client name from address → name lookup
	MacroClientName        MacroName = "{client_name}"        // Remote client hostname
	MacroClientConnections MacroName = "{client_connections}" // Connection concurrency for this client
	MacroQueueId           MacroName = "i"                    // The queue ID for this message. Some MTAs only assign a Queue ID after the DATA command (Postfix)
	MacroAuthType          MacroName = "{auth_type}"          // The used authentication method (LOGIN, DIGEST-MD5, etc)
	MacroAuthAuthen        MacroName = "{auth_authen}"        // The username of the authenticated user
	MacroAuthSsf           MacroName = "{auth_ssf}"           // The key length (in bits) of the used encryption layer (TLS) – if any
	MacroAuthAuthor        MacroName = "{auth_author}"        // The optional overwritten username for this message
	MacroMailMailer        MacroName = "{mail_mailer}"        // the delivery agent for this MAIL FROM (e.g., esmtp, lmtp)
	MacroMailHost          MacroName = "{mail_host}"          // the domain part of the MAIL FROM address
	MacroMailAddr          MacroName = "{mail_addr}"          // the MAIL FROM address (only the address without <>)
	MacroRcptMailer        MacroName = "{rcpt_mailer}"        // MacroRcptMailer holds the delivery agent/next hop for the current RCPT TO address. E.g. smtp, local.
	MacroRcptHost          MacroName = "{rcpt_host}"          // The domain part of the RCPT TO address
	MacroRcptAddr          MacroName = "{rcpt_addr}"          // the RCPT TO address (only the address without <>)
)

// Macros that do not have good cross-MTA support. Only usable with sendmail as MTA.
const (
	MacroRFC1413AuthInfo    MacroName = "_"
	MacroHopCount           MacroName = "c"
	MacroSenderHostName     MacroName = "s"
	MacroProtocolUsed       MacroName = "r"
	MacroMTAPid             MacroName = "p"
	MacroDateRFC822Origin   MacroName = "a"
	MacroDateRFC822Current  MacroName = "b"
	MacroDateANSICCurrent   MacroName = "d"
	MacroDateSecondsCurrent MacroName = "t"
)

type macroRequests [][]MacroName

type Macros interface {
	Get(name MacroName) string
	GetEx(name MacroName) (value string, ok bool)
}

// MacroBag is a default implementation of the Macros interface.
// A MacroBag is safe for concurrent use by multiple goroutines.
// It has special handling for the date-related macros and can be copied.
//
// The zero value of MacroBag is invalid. Use [NewMacroBag] to create an empty MacroBag.
type MacroBag struct {
	macros                  map[MacroName]string
	mutex                   sync.RWMutex
	currentDate, headerDate time.Time
}

func NewMacroBag() *MacroBag {
	return &MacroBag{
		macros: make(map[MacroName]string),
	}
}

func (m *MacroBag) Get(name MacroName) string {
	v, _ := m.GetEx(name)
	return v
}

func (m *MacroBag) GetEx(name MacroName) (value string, ok bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	value, ok = m.macros[name]
	if !ok {
		switch name {
		case MacroDateRFC822Origin:
			if !m.headerDate.IsZero() {
				ok = true
				value = m.headerDate.Format(time.RFC822Z)
			}
		case MacroDateRFC822Current, MacroDateSecondsCurrent, MacroDateANSICCurrent:
			ok = true
			current := m.currentDate
			if current.IsZero() {
				current = time.Now()
			}
			switch name {
			case MacroDateRFC822Current:
				value = current.Format(time.RFC822Z)
			case MacroDateSecondsCurrent:
				value = fmt.Sprintf("%d", current.Unix())
			default:
				value = current.Format(time.ANSIC)
			}
		}
	}
	return
}

func (m *MacroBag) Set(name MacroName, value string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.macros[name] = value
}

// Copy copies the macros to a new MacroBag.
// The time.Time values set by [MacroBag.SetCurrentDate] and [MacroBag.SetHeaderDate] do not get copied.
func (m *MacroBag) Copy() *MacroBag {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	macros := make(map[MacroName]string)
	for k, v := range m.macros {
		macros[k] = v
	}
	return &MacroBag{macros: macros}
}

func (m *MacroBag) SetCurrentDate(date time.Time) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.currentDate = date
}

func (m *MacroBag) SetHeaderDate(date time.Time) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.headerDate = date
}

var _ Macros = &MacroBag{}

type macrosStages struct {
	byStages []map[MacroName]string
}

func newMacroStages() *macrosStages {
	return &macrosStages{
		byStages: make([]map[MacroName]string, stageNotFoundMarker),
	}
}

func (s *macrosStages) GetMacroEx(name MacroName) (value string, stageFound MacroStage) {
	i := StageEndMarker
	for {
		if s.byStages[i] != nil {
			if v, ok := s.byStages[i][name]; ok {
				return v, i
			}
		}
		if i == StageConnect {
			return "", stageNotFoundMarker
		}
		i--
	}
}

func (s *macrosStages) SetMacro(stage MacroStage, name MacroName, value string) {
	if len(s.byStages) < int(stage) {
		panic(fmt.Sprintf("tried to set macro in invalid stage %v", stage))
	}
	if s.byStages[stage] == nil {
		s.byStages[stage] = make(map[MacroName]string)
	}
	s.byStages[stage][name] = value
}

func (s *macrosStages) SetStageMap(stage MacroStage, kv map[MacroName]string) {
	if len(s.byStages) < int(stage) {
		panic(fmt.Sprintf("tried to set invalid stage %v", stage))
	}
	s.byStages[stage] = make(map[MacroName]string)
	for k, v := range kv {
		s.byStages[stage][k] = v
	}
}

func (s *macrosStages) SetStage(stage MacroStage, kv ...string) {
	if len(kv)%2 != 0 {
		panic(fmt.Sprintf("kv needs to have an even amount of entries, not %d", len(kv)))
	}
	if len(s.byStages) < int(stage) {
		panic(fmt.Sprintf("tried to set invalid stage %v", stage))
	}
	s.byStages[stage] = make(map[MacroName]string)
	k := ""
	for i, str := range kv {
		if i%2 == 0 {
			k = str
		} else {
			s.byStages[stage][k] = str
		}
	}
}

func (s *macrosStages) DelMacro(stage MacroStage, name MacroName) {
	if s.byStages[stage] == nil {
		return
	}
	delete(s.byStages[stage], name)
	if len(s.byStages[stage]) == 0 {
		s.byStages[stage] = nil
	}
}

func (s *macrosStages) DelStage(stage MacroStage) {
	s.byStages[stage] = nil
}

func (s *macrosStages) DelStageAndAbove(stage MacroStage) {
	var stages []MacroStage
	switch stage {
	case StageConnect:
		stages = []MacroStage{StageConnect, StageHelo, StageMail, StageRcpt, StageData, StageEOH, StageEOM, StageEndMarker}
	case StageHelo:
		stages = []MacroStage{StageHelo, StageMail, StageRcpt, StageData, StageEOH, StageEOM, StageEndMarker}
	case StageMail:
		stages = []MacroStage{StageMail, StageRcpt, StageData, StageEOH, StageEOM, StageEndMarker}
	case StageRcpt:
		stages = []MacroStage{StageRcpt, StageData, StageEOH, StageEOM, StageEndMarker}
	case StageData:
		stages = []MacroStage{StageData, StageEOH, StageEOM, StageEndMarker}
	case StageEOH:
		stages = []MacroStage{StageEOH, StageEOM, StageEndMarker}
	case StageEOM:
		stages = []MacroStage{StageEOM, StageEndMarker}
	case StageEndMarker, stageNotFoundMarker:
		stages = []MacroStage{StageEndMarker}
	}
	for _, st := range stages {
		s.byStages[st] = nil
	}
}

// macroReader is a read-only Macros compatible view of its macroStages
type macroReader struct {
	macrosStages *macrosStages
}

func (r *macroReader) GetEx(name MacroName) (val string, ok bool) {
	if r == nil || r.macrosStages == nil {
		return "", false
	}
	var stage MacroStage
	val, stage = r.macrosStages.GetMacroEx(name)
	ok = stage <= StageEndMarker // StageEndMarker is for command-level macros
	return
}

func (r *macroReader) Get(name MacroName) string {
	v, _ := r.GetEx(name)
	return v
}

var _ Macros = &macroReader{}

func parseRequestedMacros(str string) []string {
	return removeEmpty(strings.FieldsFunc(str, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	}))
}

func removeEmpty(str []string) []string {
	if len(str) == 0 {
		return []string{}
	}
	indexesToKeep := make([]int, 0, len(str))
	for i, s := range str {
		if len(s) > 0 {
			indexesToKeep = append(indexesToKeep, i)
		}
	}
	r := make([]string, 0, len(indexesToKeep))
	for _, index := range indexesToKeep {
		r = append(r, str[index])
	}
	return r
}

func removeDuplicates(str []string) []string {
	if len(str) == 0 {
		return []string{}
	}
	found := make(map[string]bool, len(str))
	indexesToKeep := make([]int, 0, len(str))
	for i, v := range str {
		if !found[v] {
			indexesToKeep = append(indexesToKeep, i)
			found[v] = true
		}
	}
	noDuplicates := make([]string, len(indexesToKeep))
	for i, index := range indexesToKeep {
		noDuplicates[i] = str[index]
	}
	return noDuplicates
}
