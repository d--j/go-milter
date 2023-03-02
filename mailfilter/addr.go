package mailfilter

import (
	"strings"

	"golang.org/x/net/idna"
)

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

func (a *addr) Local() string {
	a.initParts()
	return a.parts[0]
}

func (a *addr) Domain() string {
	a.initParts()
	return a.parts[1]
}

func (a *addr) AsciiDomain() string {
	domain := a.Domain()
	if domain == "" {
		return ""
	}
	if a.asciiDomain != "" {
		return a.asciiDomain
	}

	asciiDomain, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		a.asciiDomain = domain
		return domain
	}
	a.asciiDomain = asciiDomain
	return asciiDomain
}

func (a *addr) UnicodeDomain() string {
	domain := a.Domain()
	if domain == "" {
		return ""
	}
	if a.unicodeDomain != "" {
		return a.unicodeDomain
	}

	unicodeDomain, err := idna.Lookup.ToUnicode(domain)
	if err != nil {
		a.unicodeDomain = domain
		return domain
	}
	a.unicodeDomain = unicodeDomain
	return unicodeDomain
}

type MailFrom struct {
	addr
	transport            string
	authenticatedUser    string
	authenticationMethod string
}

func (m *MailFrom) Transport() string {
	return m.transport
}

func (m *MailFrom) AuthenticatedUser() string {
	return m.authenticatedUser
}

func (m *MailFrom) AuthenticationMethod() string {
	return m.authenticationMethod
}

type RcptTo struct {
	addr
	transport string
}

func (r *RcptTo) Transport() string {
	return r.transport
}

func calculateRcptToDiff(orig []RcptTo, changed []RcptTo) (deletions []RcptTo, additions []RcptTo) {
	foundOrig := make(map[string]*RcptTo)
	foundChanged := make(map[string]bool)
	for _, r := range orig {
		foundOrig[r.Addr] = &r
	}
	for _, r := range changed {
		if o := foundOrig[r.Addr]; o == nil && !foundChanged[r.Addr] {
			additions = append(additions, r)
		} else if o != nil && o.Args != r.Args && !foundChanged[r.Addr] {
			deletions = append(deletions, *o)
			additions = append(additions, r)
		}
		foundChanged[r.Addr] = true
	}
	for _, r := range orig {
		if !foundChanged[r.Addr] {
			deletions = append(deletions, r)
		}
	}
	return
}
