// Package header includes interfaces to access and modify email headers
package header

import (
	"io"
	"time"

	"github.com/emersion/go-message/mail"
)

// Header is the interface for email headers of a mail transaction
type Header interface {
	// Add adds a new field at the end
	Add(key string, value string)
	// Value returns the value of the first field which canonical key is equal to the canonical version of key.
	// Returns the empty string when key was not found in header.
	Value(key string) string
	// UnfoldedValue returns the unfolded value (newlines replaced with spaces) of the first field which canonical key is equal to the canonical version of key.
	// Returns the empty string when key was not found in header.
	UnfoldedValue(key string) string
	// Text returns the decoded value of the first field which canonical key is equal to the canonical version of key.
	// Returns the empty string and no error when key was not found in header.
	Text(key string) (string, error)
	// AddressList returns the value interpreted as address list of the first field which canonical key is equal to the canonical version of key.
	// Returns an empty slice and no error when key was not found in header.
	AddressList(key string) ([]*mail.Address, error)
	// Set sets the value of the first header field with the canonical key "key" to "value" (as-is).
	// If key was not found, this a new header field gets added.
	// When value is the empty string, the first header field with key gets deleted.
	Set(key string, value string)
	// SetText sets the value of the first header field with the canonical key "key" to "value" (encoded).
	// If key was not found, a new header field gets added.
	SetText(key string, value string)
	// SetAddressList sets the value of the first header field with the canonical key "key" to "value" (encoded as address list).
	// The address list is encoded as multi-line header field when the MTA supports this (Sendmail does not).
	// If key was not found, a new header field gets added.
	SetAddressList(key string, addresses []*mail.Address)
	// Subject returns the decoded value of the Subject field.
	// When decoding cannot be done (e.g. because the charset is not known) the decoding error will be returned.
	// When there is no subject an empty string and no error is returned.
	Subject() (string, error)
	// SetSubject encodes the value of the Subject field.
	// When there is no subject field a new Subject field gets added.
	// When value is the empty string, the Subject field gets deleted.
	SetSubject(value string)
	// Date returns the decoded value of the Date field.
	// When decoding cannot be done (e.g. because the time cannot be parsed) the decoding error and the zero time value will be returned.
	// When there is no Date the zero [time.Time] and no error is returned.
	Date() (time.Time, error)
	// SetDate encodes the value of the Date field.
	// When there is no Date field a new Date field gets added.
	// When value is the zero [time.Time] value, the Date field gets deleted.
	SetDate(value time.Time)
	// Reader returns an [io.Reader] that produces a full properly encoded email header representation of the current fields of this header.
	// The reader includes the final CR LF sequence that separates a mail header from the body.
	Reader() io.Reader
	// Fields returns a new scanner-like iterator that iterates through all fields of this header.
	// If you modify the header fields while iterating over them (that is explicitly allowed) you should not use multiple
	// iterators of the same header at the same time.
	Fields() Fields
}

// Fields is a Scanner like interface to access all fields of a Header.
// You can modify the fields while you are iterating them.
type Fields interface {
	// Next forwards the cursor to the next field and returns true when there is a next field.
	Next() bool
	// Len returns the number of fields in the header
	Len() int
	// Raw returns the raw bytes of the current header field.
	// Panics when called before calling Next or when Next returned false.
	Raw() []byte
	// Key returns the key of the current header field as it was input.
	// Panics when called before calling Next or when Next returned false.
	Key() string
	// CanonicalKey returns the canonical version of the key of the current header field.
	// Panics when called before calling Next or when Next returned false.
	CanonicalKey() string
	// Value returns the raw value of the current header field.
	// Panics when called before calling Next or when Next returned false.
	Value() string
	// UnfoldedValue returns the unfolded value (newlines replaced with spaces) of the current header field.
	// Panics when called before calling Next or when Next returned false.
	UnfoldedValue() string
	// Text returns the decoded text of the current header field.
	// An error is returned when the text could not be decoded (e.g. because the charset is unknown).
	// Panics when called before calling Next or when Next returned false.
	Text() (string, error)
	// AddressList returns the raw bytes of the current header field.
	// Panics when called before calling Next or when Next returned false.
	AddressList() ([]*mail.Address, error)
	// Set sets the value of the current header field as-is.
	// Panics when called before calling Next or when Next returned false.
	Set(value string)
	// SetText sets the value of the current header field as encoded text.
	// Panics when called before calling Next or when Next returned false.
	SetText(value string)
	// SetAddressList sets the value of the current header field as address list value.
	// The value is encoded as multi-line header field when the MTA supports this (Sendmail does not).
	// Panics when called before calling Next or when Next returned false.
	SetAddressList(value []*mail.Address)
	// Del marks the current header field as deleted.
	// Panics when called before calling Next or when Next returned false.
	Del()
	// IsDeleted returns true when the current field is a deleted stub.
	// Panics when called before calling Next or when Next returned false.
	IsDeleted() bool
	// Replace replaces the current field with a new field with key and value (as-is).
	// Panics when called before calling Next or when Next returned false.
	Replace(key string, value string)
	// ReplaceText replaces the current field with a new field with key and value (encoded).
	// Panics when called before calling Next or when Next returned false.
	ReplaceText(key string, value string)
	// ReplaceAddressList replaces the current field with a new field with key and value.
	// The value is encoded as multi-line header field when the MTA supports this (Sendmail does not).
	// Panics when called before calling Next or when Next returned false.
	ReplaceAddressList(key string, value []*mail.Address)
	// InsertBefore adds a new field in front of the current field with key and value (as-is).
	// Panics when called before calling Next or when Next returned false.
	InsertBefore(key string, value string)
	// InsertTextBefore adds a new field in front of the current field with key and value (encoded).
	// Panics when called before calling Next or when Next returned false.
	InsertTextBefore(key string, value string)
	// InsertAddressListBefore adds a new field in front of the current field with key and value.
	// The value is encoded as multi-line header field when the MTA supports this (Sendmail does not).
	// Panics when called before calling Next or when Next returned false.
	InsertAddressListBefore(key string, value []*mail.Address)
	// InsertAfter adds a new field after the current field with key and value (as-is).
	// Panics when called before calling Next or when Next returned false.
	InsertAfter(key string, value string)
	// InsertTextAfter adds a new field after the current field with key and value (encoded).
	// Panics when called before calling Next or when Next returned false.
	InsertTextAfter(key string, value string)
	// InsertAddressListAfter adds a new field after the current field with key and value.
	// The value is encoded as multi-line header field when the MTA supports this (Sendmail does not).
	// Panics when called before calling Next or when Next returned false.
	InsertAddressListAfter(key string, value []*mail.Address)
}
