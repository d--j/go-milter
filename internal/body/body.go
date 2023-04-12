// Package body implements a write-once read-multiple [io.ReadSeekCloser] that is backed by a temporary file when too much data gets written into it.
package body

import (
	"bytes"
	"io"
	"os"
)

// New creates a new Body that switches from memory-backed storage to file-backed storage
// when more than maxMem bytes were written to it.
//
// If maxMem is less than 1 a temporary file gets always used.
func New(maxMem int) *Body {
	return &Body{maxMem: maxMem}
}

// Body is an [io.ReadSeekCloser] and [io.Writer] that starts buffering all data written to it in memory
// but when more than a configured amount of bytes is written to it Body will switch to writing to a temporary file.
//
// After a call to Read or Seek no more data can be written to Body.
// Body is an [io.Seeker] so you can read it multiple times or get the size of the Body.
type Body struct {
	maxMem  int
	buf     bytes.Buffer
	mem     *bytes.Reader
	file    *os.File
	reading bool
}

// Write implements the io.Writer interface.
// Write will create a temporary file on-the-fly when you write more than the configured amount of bytes.
func (b *Body) Write(p []byte) (n int, err error) {
	if b.reading {
		panic("cannot write after read")
	}
	if b.file != nil {
		return b.file.Write(p)
	}
	n, _ = b.buf.Write(p)
	if b.buf.Len() > b.maxMem {
		b.file, err = os.CreateTemp("", "body-*")
		if err != nil {
			return
		}
		_, err = io.Copy(b.file, &b.buf)
		b.buf.Reset()
	}
	return
}

func (b *Body) switchToReading() error {
	if !b.reading {
		b.reading = true
		if b.file != nil {
			if _, err := b.file.Seek(0, io.SeekStart); err != nil {
				return err
			}
		} else {
			b.mem = bytes.NewReader(b.buf.Bytes())
		}
	}
	return nil
}

// Read implements the io.Reader interface.
// After calling Read you cannot call Write anymore.
func (b *Body) Read(p []byte) (n int, err error) {
	if err := b.switchToReading(); err != nil {
		return 0, err
	}
	if b.file != nil {

		return b.file.Read(p)
	}
	return b.mem.Read(p)
}

// Close implements the io.Closer interface.
// If a temporary file got created it will be deleted.
func (b *Body) Close() error {
	if b.file != nil {
		err1 := b.file.Close()
		err2 := os.Remove(b.file.Name())
		if err1 != nil {
			return err1
		}
		if os.IsNotExist(err2) {
			err2 = nil
		}
		return err2
	}
	b.mem = nil
	b.buf.Reset()
	return nil
}

// Seek implements the io.Seeker interface.
// After calling Seek you cannot call Write anymore.
func (b *Body) Seek(offset int64, whence int) (int64, error) {
	if err := b.switchToReading(); err != nil {
		return 0, err
	}
	if b.file != nil {
		return b.file.Seek(offset, whence)
	}
	return b.mem.Seek(offset, whence)
}
