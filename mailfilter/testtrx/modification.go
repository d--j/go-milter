package testtrx

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

type ModificationKind int

const (
	ChangeFrom   ModificationKind = iota // a change mail from modification would have been sent to the MTA
	AddRcptTo                            // an add-recipient modification would have been sent to the MTA
	DelRcptTo                            // a delete-recipient modification would have been sent to the MTA
	InsertHeader                         // an insert-header modification would have been sent to the MTA
	ChangeHeader                         // a change-header modification would have been sent to the MTA
	ReplaceBody                          // a replace-body modification would have been sent to the MTA
)

// Modification is a modification that a [mailfilter.DecisionModificationFunc] made to the Trx.
type Modification struct {
	Kind  ModificationKind
	Addr  string
	Args  string
	Index int
	Name  string
	Value string
	Body  []byte
}

func (m *Modification) String() string {
	switch m.Kind {
	case ChangeFrom:
		return fmt.Sprintf("ChangeFrom %q %q", m.Addr, m.Args)
	case AddRcptTo:
		return fmt.Sprintf("AddRcptTo %q %q", m.Addr, m.Args)
	case DelRcptTo:
		return fmt.Sprintf("DelRcptTo %q", m.Addr)
	case InsertHeader:
		return fmt.Sprintf("InsertHeader %d %q %q", m.Index, m.Name, m.Value)
	case ChangeHeader:
		return fmt.Sprintf("ChangeHeader %d %q %q", m.Index, m.Name, m.Value)
	case ReplaceBody:
		bin := sha1.Sum(m.Body)
		hash := hex.EncodeToString(bin[:])
		return fmt.Sprintf("ReplaceBody len(body) = %d sha1(body) = %s", len(m.Body), hash)
	}
	return "Unknown modification"
}

// DiffModifications compares two slices of modifications and returns a human-readable string describing the differences.
// It returns an empty string if the slices are equal.
// If there are differences, the returned string will be formatted in unified diff format.
// The diff algorithm is very naive so you will get unnecessarily big diffs.
func DiffModifications(expected, got []Modification) string {
	sortModifications(expected)
	sortModifications(got)
	expectedLen := len(expected)
	gotLen := len(got)
	// find common prefix
	commonPrefixLen := 0
	for i := 0; i < expectedLen && i < gotLen; i++ {
		if expected[i].String() != got[i].String() {
			break
		}
		commonPrefixLen += 1
	}
	// bail if the slices are equal
	if commonPrefixLen == expectedLen && commonPrefixLen == gotLen {
		return ""
	}
	// Output the diff
	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- expected.txt\n+++ got.txt\n@@ -1,%d +1,%d @@\n", expectedLen, gotLen))
	for i := range min(expectedLen, gotLen) {
		e := expected[i].String()
		g := got[i].String()
		if e == g {
			b.WriteString(" ")
			b.WriteString(e)
		} else {
			b.WriteString("-")
			b.WriteString(e)
			b.WriteString("\n+")
			b.WriteString(g)
		}
		b.WriteString("\n")
	}
	if expectedLen > gotLen {
		for i := gotLen; i < expectedLen; i++ {
			b.WriteString("-")
			b.WriteString(expected[i].String())
			b.WriteString("\n")
		}
	}
	if gotLen > expectedLen {
		for i := expectedLen; i < gotLen; i++ {
			b.WriteString("+")
			b.WriteString(got[i].String())
			b.WriteString("\n")
		}
	}
	return b.String()
}

var sortOrder = map[ModificationKind]int{
	ChangeFrom: -10, DelRcptTo: -9, AddRcptTo: -8, InsertHeader: -7, ChangeHeader: -7, ReplaceBody: -6,
}

// sortModifications sorts the modifications in the slice without changing the semantics.
func sortModifications(mods []Modification) {
	slices.SortStableFunc(mods, func(a, b Modification) int {
		return sortOrder[a.Kind] - sortOrder[b.Kind]
	})
}
