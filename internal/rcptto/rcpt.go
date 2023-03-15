// Package rcptto includes utility functions for handling lists of recipients
package rcptto

import (
	"github.com/d--j/go-milter/mailfilter/addr"
)

// Has returns true when rcptTo is in rcptTos
func Has(rcptTos []*addr.RcptTo, rcptTo string) bool {
	findR := addr.NewRcptTo(rcptTo, "", "smtp")
	findLocal, findDomain := findR.Local(), findR.AsciiDomain()
	for _, r := range rcptTos {
		if r.Local() == findLocal && r.AsciiDomain() == findDomain {
			return true
		}
	}
	return false
}

// Add adds rcptTo with esmtpArgs to the slice rcptTos and returns the new slice.
// If rcptTo is already in rcptTos, it is not added a second time. In this case the exiting ESMTP argument gets updated.
func Add(rcptTos []*addr.RcptTo, rcptTo string, esmtpArgs string) (out []*addr.RcptTo) {
	out = rcptTos
	addR := addr.NewRcptTo(rcptTo, esmtpArgs, "new")
	findLocal, findDomain := addR.Local(), addR.AsciiDomain()
	for i, r := range out {
		if r.Local() == findLocal && r.AsciiDomain() == findDomain {
			out[i].Args = esmtpArgs
			return
		}
	}
	out = append(out, addR)
	return
}

// Del removes rcptTo from the slice rcptTos and returns the new slice.
// When rcptTo is not part of rcptTos, the slice does not get altered.
func Del(rcptTos []*addr.RcptTo, rcptTo string) (out []*addr.RcptTo) {
	out = rcptTos
	findR := addr.NewRcptTo(rcptTo, "", "")
	findLocal, findDomain := findR.Local(), findR.AsciiDomain()
	for i, r := range out {
		if r.Local() == findLocal && r.AsciiDomain() == findDomain {
			out = append(out[:i], out[i+1:]...)
			return
		}
	}
	return
}

// Copy creates an independent copy out of rcptTos
func Copy(rcptTos []*addr.RcptTo) (out []*addr.RcptTo) {
	out = make([]*addr.RcptTo, len(rcptTos))
	for i, r := range rcptTos {
		out[i] = addr.NewRcptTo(r.Addr, r.Args, r.Transport())
	}
	return
}

// Diff calculates the difference between orig and changed.
// The order of orig and change does not matter.
// A change in the ESMTP argument results in the deletion of the original RcptTo and addition of then RcptTo with the new ESMTP argument.
func Diff(orig []*addr.RcptTo, changed []*addr.RcptTo) (deletions []*addr.RcptTo, additions []*addr.RcptTo) {
	foundOrig := make(map[string]*addr.RcptTo)
	foundChanged := make(map[string]bool)
	for _, r := range orig {
		foundOrig[r.Addr] = r
	}
	for _, r := range changed {
		if o := foundOrig[r.Addr]; o == nil && !foundChanged[r.Addr] {
			additions = append(additions, r.Copy())
		} else if o != nil && o.Args != r.Args && !foundChanged[r.Addr] {
			deletions = append(deletions, o.Copy())
			additions = append(additions, r.Copy())
		}
		foundChanged[r.Addr] = true
	}
	for _, r := range orig {
		if !foundChanged[r.Addr] {
			deletions = append(deletions, r.Copy())
		}
	}
	return
}
