// Package addr includes IDNA aware address structs
package addr

import (
	"strings"

	"golang.org/x/net/idna"
)

// IDNAProfile is the [*idna.Profile] that this package uses to parse and generate the ASCII representation of domain names.
//
// This defaults to [idna.Lookup] but you can use any [*idna.Profile] you like.
var IDNAProfile = idna.Lookup

// split an user@domain address into user and domain.
// Includes the input address as third array element to quickly check if splitting must be re-done
func split(addr string) []string {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return []string{addr, "", addr}
	}

	return []string{addr[:at], addr[at+1:], addr}
}

type addr struct {
	Addr          string
	Args          string
	parts         []string
	asciiDomain   string
	unicodeDomain string
}

func (a *addr) initParts() {
	if len(a.parts) != 3 || a.parts[2] != a.Addr {
		a.parts = split(a.Addr)
		a.asciiDomain = ""
		a.unicodeDomain = ""
	}
}

// Local returns the part of an email in front of the @ symbol.
// If the address does not include an @ the whole address get returned.
func (a *addr) Local() string {
	a.initParts()
	return a.parts[0]
}

// Domain returns the part of an email after the @ symbol. It is returned as-is without any validation.
// If the address does not include an @ an empty string gets returned.
func (a *addr) Domain() string {
	a.initParts()
	return a.parts[1]
}

// AsciiDomain returns Domain interpreted and converted as the ASCII representation.
// If Domain cannot be converted (e.g. invalid UTF-8 data), the unchanged Domain value gets returned.
func (a *addr) AsciiDomain() string {
	domain := a.Domain()
	if domain == "" {
		return ""
	}
	if a.asciiDomain != "" {
		return a.asciiDomain
	}

	asciiDomain, err := IDNAProfile.ToASCII(domain)
	if err != nil {
		a.asciiDomain = domain
		return domain
	}
	a.asciiDomain = asciiDomain
	return asciiDomain
}

// UnicodeDomain returns Domain interpreted and converted as the UTF-8 representation.
// If Domain cannot be converted (e.g. invalid UTF-8 data), the unchanged Domain value gets returned.
func (a *addr) UnicodeDomain() string {
	domain := a.Domain()
	if domain == "" {
		return ""
	}
	if a.unicodeDomain != "" {
		return a.unicodeDomain
	}

	unicodeDomain, err := IDNAProfile.ToUnicode(domain)
	if err != nil {
		a.unicodeDomain = domain
		return domain
	}
	a.unicodeDomain = unicodeDomain
	return unicodeDomain
}

// MailFrom is the sender address and the sender info (used transport, authenticated user).
type MailFrom struct {
	addr
	transport            string
	authenticatedUser    string
	authenticationMethod string
}

// NewMailFrom creates a new [MailFrom]
func NewMailFrom(from, esmtpArgs, transport, authenticatedUser, authenticationMethod string) MailFrom {
	return MailFrom{
		addr:                 addr{Addr: from, Args: esmtpArgs},
		transport:            transport,
		authenticatedUser:    authenticatedUser,
		authenticationMethod: authenticationMethod,
	}
}

// Transport returns the used transport. You might use this to e.g. distinguish local generated mail from incoming mail.
func (m *MailFrom) Transport() string {
	return m.transport
}

// AuthenticatedUser is the username of the logged-in user. It is empty, when there is no login.
func (m *MailFrom) AuthenticatedUser() string {
	return m.authenticatedUser
}

// AuthenticationMethod is the used method of authentication. E.g. "PLAIN" or "CRAM-MD5". It is empty, when there is no login.
func (m *MailFrom) AuthenticationMethod() string {
	return m.authenticationMethod
}

// Copy returns an independent copy of m.
func (m *MailFrom) Copy() *MailFrom {
	if m == nil {
		return nil
	}
	return &MailFrom{
		addr:                 addr{Addr: m.Addr, Args: m.Args},
		transport:            m.transport,
		authenticatedUser:    m.authenticatedUser,
		authenticationMethod: m.authenticationMethod,
	}
}

// RcptTo is one recipient address and its metadata.
type RcptTo struct {
	addr
	transport string
}

// NewRcptTo creates a new [RcptTo]
func NewRcptTo(to, esmtpArgs, transport string) *RcptTo {
	return &RcptTo{
		addr:      addr{Addr: to, Args: esmtpArgs},
		transport: transport,
	}
}

// Transport returns the next-hop transport . You might use this to e.g. distinguish a local recipient from an external recipient.
func (r *RcptTo) Transport() string {
	return r.transport
}

// Copy returns an independent copy of r.
func (r *RcptTo) Copy() *RcptTo {
	if r == nil {
		return nil
	}
	return &RcptTo{
		addr:      addr{Addr: r.Addr, Args: r.Args},
		transport: r.transport,
	}
}
