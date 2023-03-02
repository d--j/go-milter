package mailfilter

import (
	"io"
	netMail "net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
)

const helperKey = "Helper"
const dateLayout = "Mon, 02 Jan 2006 15:04:05 -0700"

func newHelper() *mail.Header {
	helper := mail.HeaderFromMap(map[string][]string{helperKey: {" "}})
	return &helper
}

type headerField struct {
	index        int
	canonicalKey string
	raw          string
}

func (f *headerField) key() string {
	return f.raw[:len(f.canonicalKey)]
}

func (f *headerField) value() string {
	return f.raw[len(f.canonicalKey)+1:]
}

func (f *headerField) deleted() bool {
	return len(f.raw) <= len(f.canonicalKey)+1
}

type Header struct {
	fields []*headerField
	helper *mail.Header
}

func (h *Header) copy() *Header {
	h2 := Header{}
	h2.fields = make([]*headerField, len(h.fields))
	for i, f := range h.fields {
		c := *f
		h2.fields[i] = &c
	}
	return &h2
}

func (h *Header) addRaw(key string, raw string) {
	h.fields = append(h.fields, &headerField{len(h.fields), textproto.CanonicalMIMEHeaderKey(key), raw})
}

func (h *Header) Add(key string, value string) {
	h.fields = append(h.fields, &headerField{-1, textproto.CanonicalMIMEHeaderKey(key), getRaw(key, value)})
}

func (h *Header) Get(key string) string {
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for _, f := range h.fields {
		if f.canonicalKey == canonicalKey {
			return f.value()
		}
	}
	return ""
}

func (h *Header) GetText(key string) (string, error) {
	if h.helper == nil {
		h.helper = newHelper()
	}
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for _, f := range h.fields {
		if f.canonicalKey == canonicalKey {
			h.helper.Set(helperKey, f.value())
			return h.helper.Text(helperKey)
		}
	}
	return "", nil
}

func (h *Header) GetAddressList(key string) ([]*mail.Address, error) {
	if h.helper == nil {
		h.helper = newHelper()
	}
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for _, f := range h.fields {
		if f.canonicalKey == canonicalKey {
			h.helper.Set(helperKey, f.value())
			return h.helper.AddressList(helperKey)
		}
	}
	return []*mail.Address{}, nil
}

func (h *Header) Set(key string, value string) {
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for i := range h.fields {
		if h.fields[i].canonicalKey == canonicalKey {
			h.fields[i] = &headerField{
				index:        h.fields[i].index,
				canonicalKey: canonicalKey,
				raw:          getRaw(h.fields[i].key(), value),
			}
			return
		}
	}
	if value != "" {
		h.Add(key, value)
	}
}

func (h *Header) SetText(key string, value string) {
	if h.helper == nil {
		h.helper = newHelper()
	}
	h.helper.SetText(helperKey, value)
	h.Set(key, h.helper.Get(helperKey))
}

func (h *Header) SetAddressList(key string, addresses []*mail.Address) {
	if h.helper == nil {
		h.helper = newHelper()
	}
	h.helper.SetAddressList(helperKey, addresses)
	h.Set(key, h.helper.Get(helperKey))
}

func (h *Header) Subject() (string, error) {
	return h.GetText("Subject")
}

func (h *Header) SetSubject(value string) {
	h.SetText("Subject", value)
}

func (h *Header) Date() (time.Time, error) {
	return netMail.ParseDate(h.Get("Date"))
}

// SetDate sets the Date header to the value.
// The zero value of [time.Time] as valid. This will delete the Date header when it exists.
func (h *Header) SetDate(value time.Time) {
	if value.IsZero() {
		h.Set("Date", "")
	} else {
		h.Set("Date", value.Format(dateLayout))
	}
}

func (h *Header) Fields() *HeaderFields {
	return &HeaderFields{
		cursor: -1,
		skip:   0,
		h:      h,
		helper: newHelper(),
	}
}

func (h *Header) Reader() io.Reader {
	const crlf = "\r\n"
	readers := make([]io.Reader, 0, len(h.fields)*2+1)
	for _, f := range h.fields {
		if !f.deleted() { // skip deleted
			readers = append(readers, strings.NewReader(f.raw))
			readers = append(readers, strings.NewReader(crlf))
		}
	}
	readers = append(readers, strings.NewReader(crlf))
	return io.MultiReader(readers...)
}

type HeaderFields struct {
	cursor int
	skip   int
	h      *Header
	helper *mail.Header
}

func (f *HeaderFields) Next() bool {
	f.cursor += f.skip // skip the InsertAfter headers
	f.skip = 0
	f.cursor += 1
	return f.cursor < len(f.h.fields)
}

// Len returns the number of fields in the header.
// This also includes deleted headers fields.
// Initially no fields are deleted so Len returns the actual number of header fields.
func (f *HeaderFields) Len() int {
	return len(f.h.fields)
}

func (f *HeaderFields) index() int {
	if f.cursor < 0 || f.cursor >= len(f.h.fields) {
		panic("index called before call to Next() or after Next() returned false")
	}
	return f.cursor
}

func (f *HeaderFields) raw() string {
	return f.h.fields[f.index()].raw
}

func (f *HeaderFields) Key() string {
	return f.h.fields[f.index()].key()
}

func (f *HeaderFields) CanonicalKey() string {
	return f.h.fields[f.index()].canonicalKey
}

// IsDeleted returns true when a previous header modification deleted this header.
// You can "undelete" the header by just calling [HeaderFields.Set] with a non-empty value.
func (f *HeaderFields) IsDeleted() bool {
	return f.h.fields[f.index()].deleted()
}

func (f *HeaderFields) Get() string {
	return f.h.fields[f.index()].value()
}

func (f *HeaderFields) GetText() (string, error) {
	f.helper.Set(helperKey, f.Get())
	return f.helper.Text(helperKey)
}

func (f *HeaderFields) GetAddressList() ([]*mail.Address, error) {
	f.helper.Set(helperKey, f.Get())
	return f.helper.AddressList(helperKey)
}

func getRaw(key string, value string) string {
	if len(value) > 0 && !(value[0] == ' ' || value[0] == '\t') {
		return key + ": " + value
	} else {
		return key + ":" + value
	}
}

func (f *HeaderFields) Set(value string) {
	idx := f.index()
	f.h.fields[idx] = &headerField{f.h.fields[idx].index, f.CanonicalKey(), getRaw(f.Key(), value)}
}

func (f *HeaderFields) text(value string) string {
	f.helper.SetText(helperKey, value)
	return f.helper.Get(helperKey)
}

func (f *HeaderFields) SetText(value string) {
	f.Set(f.text(value))
}

func (f *HeaderFields) addressList(value []*mail.Address) string {
	f.helper.SetAddressList(helperKey, value)
	return f.helper.Get(helperKey)
}

func (f *HeaderFields) SetAddressList(value []*mail.Address) {
	f.Set(f.addressList(value))
}

func (f *HeaderFields) Del() {
	f.Set("")
}

func (f *HeaderFields) Replace(key string, value string) {
	idx := f.index()
	f.h.fields[idx] = &headerField{f.h.fields[idx].index, textproto.CanonicalMIMEHeaderKey(key), getRaw(key, value)}
}

func (f *HeaderFields) ReplaceText(key string, value string) {
	f.Replace(key, f.text(value))
}

func (f *HeaderFields) ReplaceAddressList(key string, value []*mail.Address) {
	f.Replace(key, f.addressList(value))
}

func (f *HeaderFields) insert(index int, key string, value string) {
	tail := make([]*headerField, 1, 1+len(f.h.fields)-index)
	tail[0] = &headerField{-1, textproto.CanonicalMIMEHeaderKey(key), getRaw(key, value)}
	tail = append(tail, f.h.fields[index:]...)
	f.h.fields = append(f.h.fields[:index], tail...)
}

func (f *HeaderFields) InsertBefore(key string, value string) {
	f.insert(f.index(), key, value)
	f.cursor += 1
}

func (f *HeaderFields) InsertTextBefore(key string, value string) {
	f.InsertBefore(key, f.text(value))
}

func (f *HeaderFields) InsertAddressListBefore(key string, value []*mail.Address) {
	f.InsertBefore(key, f.addressList(value))
}

func (f *HeaderFields) InsertAfter(key string, value string) {
	f.skip += 1
	f.insert(f.index()+f.skip, key, value)
}

func (f *HeaderFields) InsertTextAfter(key string, value string) {
	f.InsertAfter(key, f.text(value))
}

func (f *HeaderFields) InsertAddressListAfter(key string, value []*mail.Address) {
	f.InsertAfter(key, f.addressList(value))
}

const (
	kindEqual = iota
	kindChange
	kindInsert
)

type headerFieldDiff struct {
	kind  int
	field *headerField
	index int
}

func diffHeaderFieldsMiddle(orig []*headerField, changed []*headerField, index int) (diffs []headerFieldDiff) {
	// either orig and changed are empty or the first element is different
	origLen, changedLen := len(orig), len(changed)
	changedI := 0
	switch {
	case origLen == 0 && changedLen == 0:
		return nil
	case origLen == 0:
		// orig empty -> everything must be inserts
		for _, c := range changed {
			diffs = append(diffs, headerFieldDiff{kindInsert, c, index})
		}
		return
	case changedLen == 0:
		// This should not happen since we do not delete headerField entries
		// but if the user completely replaces the headers it could indeed happen.
		// Panic in this case so the programming error surfaces.
		panic("internal structure error: do not completely replace transaction.Headers – use its methods to alter it")
	default: // origLen > 0 && changedLen > 0
		o := orig[0]
		if o.index < 0 {
			panic("internal structure error: all elements in orig need to have an index bigger than -1: do not completely replace transaction.Headers – use its methods to alter it")
		}
		found := false
		// find o in changed
		for i, c := range changed {
			if c.index == o.index {
				found = true
				index = o.index
				changedI = i
				for i = 0; i < changedI; i++ {
					diffs = append(diffs, headerFieldDiff{kindInsert, changed[i], index - 1})
				}
				if changed[changedI].raw == o.raw {
					diffs = append(diffs, headerFieldDiff{kindEqual, o, o.index})
				} else if changed[changedI].key() == o.key() {
					diffs = append(diffs, headerFieldDiff{kindChange, changed[changedI], o.index})
				} else {
					// a HeaderFields.Replace call, delete the original
					diffs = append(diffs, headerFieldDiff{
						kind: kindChange,
						field: &headerField{
							index:        o.index,
							canonicalKey: o.canonicalKey,
							raw:          o.key() + ":",
						},
						index: o.index,
					})
					// insert changed in front of deleted header
					diffs = append(diffs, headerFieldDiff{kindInsert, &headerField{
						index:        -1,
						canonicalKey: changed[changedI].canonicalKey,
						raw:          changed[changedI].raw,
					}, index})
					index-- // in this special case we actually do not need to increase the index below
				}
				changedI++
				break
			} else if c.index > o.index {
				break
			}
		}
		// if o not in changed we need to delete it
		if !found {
			diffs = append(diffs, headerFieldDiff{
				kind: kindChange,
				field: &headerField{
					index:        o.index,
					canonicalKey: o.canonicalKey,
					raw:          o.key() + ":",
				},
				index: o.index,
			})
		}
		// we only consumed the first element of orig
		index++
		restDiffs := diffHeaderFields(orig[1:], changed[changedI:], index)
		if len(restDiffs) > 0 {
			diffs = append(diffs, restDiffs...)
		}
		return
	}
}

func diffHeaderFields(orig []*headerField, changed []*headerField, index int) (diffs []headerFieldDiff) {
	origLen, changedLen := len(orig), len(changed)
	// find common prefix
	commonPrefixLen, commonSuffixLen := 0, 0
	for i := 0; i < origLen && i < changedLen; i++ {
		if orig[i].raw != changed[i].raw || orig[i].index != changed[i].index {
			break
		}
		commonPrefixLen += 1
		index = orig[i].index
	}
	// find common suffix (down to the commonPrefixLen element)
	i, j := origLen-1, changedLen-1
	for i > commonPrefixLen-1 && j > commonPrefixLen-1 {
		if orig[i].raw != changed[j].raw || orig[i].index != changed[j].index {
			break
		}
		commonSuffixLen += 1
		i--
		j--
	}
	for i := 0; i < commonPrefixLen; i++ {
		diffs = append(diffs, headerFieldDiff{kindEqual, orig[i], orig[i].index})
	}
	// find the changed parts, recursively calls diffHeaderFields afterwards
	middleDiffs := diffHeaderFieldsMiddle(orig[commonPrefixLen:origLen-commonSuffixLen], changed[commonPrefixLen:changedLen-commonSuffixLen], index)
	if len(middleDiffs) > 0 {
		diffs = append(diffs, middleDiffs...)
	}
	for i := origLen - commonSuffixLen; i < origLen; i++ {
		diffs = append(diffs, headerFieldDiff{kindEqual, orig[i], orig[i].index})
	}
	return
}

type headerOp struct {
	Index int
	Name  string
	Value string
}

// calculateHeaderModifications finds differences between orig and changed.
// The differences are expressed as change and insert operations – to be mapped to milter modification actions.
// Deletions are changes to an empty value.
func calculateHeaderModifications(orig *Header, changed *Header) (changeOps []headerOp, insertOps []headerOp) {
	origFields := orig.Fields()
	origLen := origFields.Len()
	origIndexByKeyCounter := make(map[string]int)
	origIndexByKey := make([]int, origLen)
	for i := 0; origFields.Next(); i++ {
		origIndexByKeyCounter[origFields.CanonicalKey()] += 1
		origIndexByKey[i] = origIndexByKeyCounter[origFields.CanonicalKey()]
	}
	diffs := diffHeaderFields(orig.fields, changed.fields, -1)
	for _, diff := range diffs {
		switch diff.kind {
		case kindInsert:
			insertOps = append(insertOps, headerOp{
				Index: diff.index + 1,
				Name:  diff.field.key(),
				Value: diff.field.value(),
			})
		case kindChange:
			if diff.index < origLen {
				changeOps = append(changeOps, headerOp{
					Index: origIndexByKey[diff.index],
					Name:  diff.field.key(),
					Value: diff.field.value(),
				})
			} else { // should not happen but just make inserts out of it
				insertOps = append(insertOps, headerOp{
					Index: diff.index + 1,
					Name:  diff.field.key(),
					Value: diff.field.value(),
				})
			}
		}
	}

	return
}
