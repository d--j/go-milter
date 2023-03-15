package addr

import (
	"reflect"
	"testing"
	"unsafe"
)

func Test_addr_AsciiDomain(t *testing.T) {
	tests := []struct {
		name string
		Addr string
		want string
	}{
		{"empty", "", ""},
		{"no domain", "root", ""},
		{"normal", "root@localhost", "localhost"},
		{"IDNA", "root@スパム.example.com", "xn--zck5b2b.example.com"},
		{"IDNA encoded", "root@xn--zck5b2b.example.com", "xn--zck5b2b.example.com"},
		{"IDNA broken", "root@スパム\u0000\u0000\u0000\u0000.example.com", "スパム\u0000\u0000\u0000\u0000.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := addr{
				Addr: tt.Addr,
			}
			if got := a.AsciiDomain(); got != tt.want {
				t.Errorf("AsciiDomain() = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("cache", func(t *testing.T) {
		a := addr{
			Addr: "root@localhost",
		}
		got1 := a.AsciiDomain()
		got2 := a.AsciiDomain()

		hdr1 := (*reflect.StringHeader)(unsafe.Pointer(&got1))
		hdr2 := (*reflect.StringHeader)(unsafe.Pointer(&got2))

		if hdr1.Data != hdr2.Data {
			t.Errorf("AsciiDomain() did not cache value")
		}
	})
}

func Test_addr_Domain(t *testing.T) {
	tests := []struct {
		name string
		Addr string
		want string
	}{
		{"empty", "", ""},
		{"no domain", "root", ""},
		{"normal", "root@localhost", "localhost"},
		{"IDNA", "root@スパム.example.com", "スパム.example.com"},
		{"IDNA encoded", "root@xn--zck5b2b.example.com", "xn--zck5b2b.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := addr{
				Addr: tt.Addr,
			}
			if got := a.Domain(); got != tt.want {
				t.Errorf("Domain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addr_Local(t *testing.T) {
	tests := []struct {
		name string
		Addr string
		want string
	}{
		{"empty", "", ""},
		{"no domain", "root", "root"},
		{"normal", "root@localhost", "root"},
		{"IDNA", "root@スパム.example.com", "root"},
		{"IDNA encoded", "root@xn--zck5b2b.example.com", "root"},
		{"bogus", "local root@localhost", "local root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := addr{
				Addr: tt.Addr,
			}
			if got := a.Local(); got != tt.want {
				t.Errorf("Local() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addr_UnicodeDomain(t *testing.T) {
	tests := []struct {
		name string
		Addr string
		want string
	}{
		{"empty", "", ""},
		{"no domain", "root", ""},
		{"normal", "root@localhost", "localhost"},
		{"IDNA", "root@スパム.example.com", "スパム.example.com"},
		{"IDNA encoded", "root@xn--zck5b2b.example.com", "スパム.example.com"},
		{"IDNA broken", "root@xn--zck5b2b\u0000\u0000\u0000\u0000.example.com", "xn--zck5b2b\u0000\u0000\u0000\u0000.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := addr{
				Addr: tt.Addr,
			}
			if got := a.UnicodeDomain(); got != tt.want {
				t.Errorf("UnicodeDomain() = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("cache", func(t *testing.T) {
		a := addr{
			Addr: "root@localhost",
		}
		got1 := a.UnicodeDomain()
		got2 := a.UnicodeDomain()

		hdr1 := (*reflect.StringHeader)(unsafe.Pointer(&got1))
		hdr2 := (*reflect.StringHeader)(unsafe.Pointer(&got2))

		if hdr1.Data != hdr2.Data {
			t.Errorf("UnicodeDomain() did not cache value")
		}
	})
}

func TestMailFrom(t *testing.T) {
	m := MailFrom{
		addr:                 addr{Addr: "root@localhost", Args: "A=B"},
		transport:            "smtpd",
		authenticatedUser:    "root",
		authenticationMethod: "PLAIN",
	}
	if v := m.Transport(); v != "smtpd" {
		t.Errorf("Transoprt() = %q, want %q", v, "smtpd")
	}
	if v := m.AuthenticatedUser(); v != "root" {
		t.Errorf("AuthenticatedUser() = %q, want %q", v, "root")
	}
	if v := m.AuthenticationMethod(); v != "PLAIN" {
		t.Errorf("AuthenticationMethod() = %q, want %q", v, "PLAIN")
	}
}

func TestRcptTo(t *testing.T) {
	m := RcptTo{
		addr:      addr{Addr: "root@localhost", Args: "A=B"},
		transport: "lmtp",
	}
	if v := m.Transport(); v != "lmtp" {
		t.Errorf("Transoprt() = %q, want %q", v, "lmtp")
	}
}

func Test_split(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want []string
	}{
		{"empty", "", []string{"", "", ""}},
		{"no domain", "root", []string{"root", "", "root"}},
		{"normal", "root@localhost", []string{"root", "localhost", "root@localhost"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := split(tt.addr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("split() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewMailFrom(t *testing.T) {
	type args struct {
		from                 string
		esmtpArgs            string
		transport            string
		authenticatedUser    string
		authenticationMethod string
	}
	tests := []struct {
		name string
		args args
		want MailFrom
	}{
		{"works", args{"root", "A=B", "smtp", "user", "method"}, MailFrom{addr: addr{Addr: "root", Args: "A=B"}, transport: "smtp", authenticatedUser: "user", authenticationMethod: "method"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMailFrom(tt.args.from, tt.args.esmtpArgs, tt.args.transport, tt.args.authenticatedUser, tt.args.authenticationMethod); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewMailFrom() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMailFrom_Copy(t *testing.T) {
	t.Parallel()
	null := (*MailFrom)(nil)
	if got := null.Copy(); got != nil {
		t.Errorf("Copy(nil) = %v, want %v", got, nil)
	}
	r1 := NewMailFrom("root", "", "", "", "")
	got := r1.Copy()
	if got == &r1 {
		t.Errorf("Copy() did not create an independent copy")
	}
}

func TestNewRcptTo(t *testing.T) {
	type args struct {
		to        string
		esmtpArgs string
		transport string
	}
	tests := []struct {
		name string
		args args
		want *RcptTo
	}{
		{"works", args{"root", "A=B", "smtp"}, &RcptTo{addr: addr{Addr: "root", Args: "A=B"}, transport: "smtp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewRcptTo(tt.args.to, tt.args.esmtpArgs, tt.args.transport); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewRcptTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRcptTo_Copy(t *testing.T) {
	t.Parallel()
	null := (*RcptTo)(nil)
	if got := null.Copy(); got != nil {
		t.Errorf("Copy(nil) = %v, want %v", got, nil)
	}
	r1 := NewRcptTo("root", "", "")
	got := r1.Copy()
	if got == r1 {
		t.Errorf("Copy() did not create an independent copy")
	}
}
