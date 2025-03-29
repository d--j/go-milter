// Package header has structs and functions handling with mail header and their modifications
package header

import (
	"bytes"
	"github.com/emersion/go-message"
	"io"
	netmail "net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"

	"github.com/d--j/go-milter/mailfilter/header"
	"github.com/emersion/go-message/mail"
)

var unfoldRegex = regexp.MustCompile(`\r?\n\s*`)

func unfold(lines string) string {
	return unfoldRegex.ReplaceAllString(lines, " ")
}

func formatAddressList(l []*mail.Address) string {
	formatted := make([]string, len(l))
	for i, a := range l {
		formatted[i] = a.String()
	}
	return strings.Join(formatted, ",\r\n ")
}

type Field struct {
	Index        int
	CanonicalKey string
	Raw          []byte
	deleted      bool
}

func (f *Field) Key() string {
	return string(f.Raw[:len(f.CanonicalKey)])
}

func (f *Field) Value() string {
	return string(f.Raw[len(f.CanonicalKey)+1:])
}

func (f *Field) UnfoldedValue() string {
	return unfold(string(f.Raw[len(f.CanonicalKey)+1:]))
}

func (f *Field) Deleted() bool {
	return f.deleted
}

const helperKey = "Helper"
const dateLayout = "Mon, 02 Jan 2006 15:04:05 -0700"

func newHelper() *mail.Header {
	helper := mail.HeaderFromMap(map[string][]string{helperKey: {" "}})
	return &helper
}

type Header struct {
	fields []*Field
	helper *mail.Header
}

func New(raw []byte) (*Header, error) {
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		if message.IsUnknownCharset(err) {
			err = nil
		} else {
			return nil, err
		}
	}
	f := r.Header.Fields()
	h := Header{}
	h.fields = make([]*Field, f.Len())
	for i := 0; f.Next(); i++ {
		b, err := f.Raw()
		if err != nil {
			return nil, err
		}
		h.fields[i] = &Field{
			Index:        i,
			CanonicalKey: textproto.CanonicalMIMEHeaderKey(f.Key()),
			Raw:          b[:len(b)-2],
		}
	}
	return &h, nil
}

func (h *Header) Copy() *Header {
	h2 := Header{}
	h2.fields = make([]*Field, len(h.fields))
	for i, f := range h.fields {
		c := *f
		h2.fields[i] = &c
	}
	return &h2
}

func (h *Header) AddRaw(key string, raw []byte) {
	h.fields = append(h.fields, &Field{len(h.fields), textproto.CanonicalMIMEHeaderKey(key), raw, false})
}

func (h *Header) Add(key string, value string) {
	h.fields = append(h.fields, &Field{-1, textproto.CanonicalMIMEHeaderKey(key), getRaw(key, value), false})
}

func (h *Header) Value(key string) string {
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for _, f := range h.fields {
		if f.CanonicalKey == canonicalKey && !f.Deleted() {
			return f.Value()
		}
	}
	return ""
}

func (h *Header) UnfoldedValue(key string) string {
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for _, f := range h.fields {
		if f.CanonicalKey == canonicalKey && !f.Deleted() {
			return f.UnfoldedValue()
		}
	}
	return ""
}

func (h *Header) Text(key string) (string, error) {
	if h.helper == nil {
		h.helper = newHelper()
	}
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for _, f := range h.fields {
		if f.CanonicalKey == canonicalKey && !f.Deleted() {
			h.helper.Set(helperKey, f.UnfoldedValue())
			return h.helper.Text(helperKey)
		}
	}
	return "", nil
}

func (h *Header) AddressList(key string) ([]*mail.Address, error) {
	if h.helper == nil {
		h.helper = newHelper()
	}
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for _, f := range h.fields {
		if f.CanonicalKey == canonicalKey && !f.Deleted() {
			h.helper.Set(helperKey, f.UnfoldedValue())
			return h.helper.AddressList(helperKey)
		}
	}
	return []*mail.Address{}, nil
}

func (h *Header) Set(key string, value string) {
	canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
	for i := range h.fields {
		if h.fields[i].CanonicalKey == canonicalKey {
			h.fields[i] = &Field{
				Index:        h.fields[i].Index,
				CanonicalKey: canonicalKey,
				Raw:          getRaw(h.fields[i].Key(), value),
				deleted:      value == "",
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
	h.Set(key, formatAddressList(addresses))
}

func (h *Header) Subject() (string, error) {
	return h.Text("Subject")
}

func (h *Header) SetSubject(value string) {
	h.SetText("Subject", value)
}

func (h *Header) Date() (time.Time, error) {
	return netmail.ParseDate(h.Value("Date"))
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

func (h *Header) Fields() header.Fields {
	return &Fields{
		cursor: -1,
		skip:   0,
		h:      h,
		helper: newHelper(),
	}
}

func (h *Header) Reader() io.Reader {
	const crlf = "\r\n"
	readers := make([]io.Reader, 0, h.Len()*2+1)
	for _, f := range h.fields {
		if !f.Deleted() { // skip deleted
			readers = append(readers, bytes.NewReader(f.Raw))
			readers = append(readers, strings.NewReader(crlf))
		}
	}
	readers = append(readers, strings.NewReader(crlf))
	return io.MultiReader(readers...)
}

// Len returns the number of fields in the header. It includes deleted fields.
func (h *Header) Len() int {
	return len(h.fields)
}

var _ header.Header = (*Header)(nil)

type Fields struct {
	cursor int
	skip   int
	h      *Header
	helper *mail.Header
}

func (f *Fields) Next() bool {
	f.cursor += f.skip // skip the InsertAfter headers
	f.skip = 0
	f.cursor += 1
	return f.h == nil || f.cursor < len(f.h.fields)
}

// Len returns the number of fields in the header.
// This also includes deleted headers fields.
// Initially no fields are deleted so Len returns the actual number of header fields.
func (f *Fields) Len() int {
	if f == nil || f.h == nil {
		return 0
	}
	return len(f.h.fields)
}

func (f *Fields) index() int {
	if f.cursor < 0 || f.cursor >= len(f.h.fields) {
		panic("index called before call to Next() or after Next() returned false")
	}
	return f.cursor
}

func (f *Fields) Raw() []byte {
	return f.h.fields[f.index()].Raw
}

func (f *Fields) Key() string {
	return f.h.fields[f.index()].Key()
}

func (f *Fields) CanonicalKey() string {
	return f.h.fields[f.index()].CanonicalKey
}

// IsDeleted returns true when a previous header modification deleted this header.
// You can "undelete" the header by just calling [HeaderFields.Set] with a non-empty value.
func (f *Fields) IsDeleted() bool {
	return f.h.fields[f.index()].Deleted()
}

func (f *Fields) Value() string {
	return f.h.fields[f.index()].Value()
}

func (f *Fields) UnfoldedValue() string {
	return f.h.fields[f.index()].UnfoldedValue()
}

func (f *Fields) Text() (string, error) {
	f.helper.Set(helperKey, f.UnfoldedValue())
	return f.helper.Text(helperKey)
}

func (f *Fields) AddressList() ([]*mail.Address, error) {
	f.helper.Set(helperKey, f.UnfoldedValue())
	return f.helper.AddressList(helperKey)
}

func getRaw(key string, value string) []byte {
	if len(value) > 0 && !(value[0] == ' ' || value[0] == '\t') {
		return []byte(key + ": " + value)
	} else {
		return []byte(key + ":" + value)
	}
}

func (f *Fields) Set(value string) {
	idx := f.index()
	f.h.fields[idx] = &Field{f.h.fields[idx].Index, f.CanonicalKey(), getRaw(f.Key(), value), value == ""}
}

func (f *Fields) text(value string) string {
	f.helper.SetText(helperKey, value)
	return f.helper.Get(helperKey)
}

func (f *Fields) SetText(value string) {
	f.Set(f.text(value))
}

func (f *Fields) addressList(value []*mail.Address) string {
	return formatAddressList(value)
}

func (f *Fields) SetAddressList(value []*mail.Address) {
	f.Set(f.addressList(value))
}

func (f *Fields) Del() {
	f.Set("")
}

func (f *Fields) Replace(key string, value string) {
	idx := f.index()
	f.h.fields[idx] = &Field{f.h.fields[idx].Index, textproto.CanonicalMIMEHeaderKey(key), getRaw(key, value), false}
}

func (f *Fields) ReplaceText(key string, value string) {
	f.Replace(key, f.text(value))
}

func (f *Fields) ReplaceAddressList(key string, value []*mail.Address) {
	f.Replace(key, f.addressList(value))
}

func (f *Fields) insert(index int, key string, value string) {
	tail := make([]*Field, 1, 1+len(f.h.fields)-index)
	tail[0] = &Field{-1, textproto.CanonicalMIMEHeaderKey(key), getRaw(key, value), false}
	tail = append(tail, f.h.fields[index:]...)
	f.h.fields = append(f.h.fields[:index], tail...)
}

func (f *Fields) InsertBefore(key string, value string) {
	f.insert(f.index(), key, value)
	f.cursor += 1
}

func (f *Fields) InsertTextBefore(key string, value string) {
	f.InsertBefore(key, f.text(value))
}

func (f *Fields) InsertAddressListBefore(key string, value []*mail.Address) {
	f.InsertBefore(key, f.addressList(value))
}

func (f *Fields) InsertAfter(key string, value string) {
	f.skip += 1
	f.insert(f.index()+f.skip, key, value)
}

func (f *Fields) InsertTextAfter(key string, value string) {
	f.InsertAfter(key, f.text(value))
}

func (f *Fields) InsertAddressListAfter(key string, value []*mail.Address) {
	f.InsertAfter(key, f.addressList(value))
}

var _ header.Fields = (*Fields)(nil)
