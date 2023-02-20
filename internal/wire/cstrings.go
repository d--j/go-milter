package wire

import (
	"bytes"
	"strings"
)

// NULL terminator
const null = "\x00"

// DecodeCStrings splits a C style strings into a Go string slice
// The last C style string in data can optionally not be terminated with a null-byte.
func DecodeCStrings(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	// strip the last null byte
	if data[len(data)-1] == 0 {
		data = data[0 : len(data)-1]
	}
	return strings.Split(string(data), null)
}

// ReadCString reads and returns a C style string from []byte.
// If data does not contain a null-byte the whole data-slice is returned as string
func ReadCString(data []byte) string {
	pos := bytes.IndexByte(data, 0)
	if pos == -1 {
		return string(data)
	}
	return string(data[0:pos])
}

// AppendCString appends a C style string to the buffer and returns it (like append does).
// It is assumed that s does not contain null-bytes.
func AppendCString(dest []byte, s string) []byte {
	dest = append(dest, []byte(s)...)
	dest = append(dest, 0x00)
	return dest
}
