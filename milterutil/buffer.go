// Package milterutil includes utility functions and types that might be useful for writing milters or MTAs.
package milterutil

import (
	"bufio"
	"io"
	"sync"
)

// FixedBufferScanner is a wrapper around a [bufio.Scanner] that produces fixed size chunks of data
// given an [io.Reader].
type FixedBufferScanner struct {
	bufferSize uint32
	buffer     []byte
	scanner    *bufio.Scanner
	pool       *sync.Pool
}

func (f *FixedBufferScanner) init(pool *sync.Pool, r io.Reader) {
	var bufSize = int(f.bufferSize)
	f.pool = pool
	f.scanner = bufio.NewScanner(r)
	f.scanner.Buffer(f.buffer, bufSize)
	f.scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		// buffer full? Return it.
		if len(data) >= bufSize {
			return bufSize, data[0:bufSize], nil
		}
		// If we're at EOF, return the rest even if it is less than bufSize
		if atEOF {
			return len(data), data, nil
		}
		// Request more data.
		return 0, nil, nil
	})
}

// Scan returns true when there is new data in Bytes
func (f *FixedBufferScanner) Scan() bool {
	return f.scanner.Scan()
}

// Bytes returns the current chunk of data
func (f *FixedBufferScanner) Bytes() []byte {
	return f.scanner.Bytes()
}

// Err returns the first non-EOF error encountered by the FixedBufferScanner.
func (f *FixedBufferScanner) Err() error {
	return f.scanner.Err()
}

// Close need to be called when you are done with the FixedBufferScanner because we maintain a shared pool
// of FixedBufferScanner objects.
//
// Close does not close the underlying [io.Reader]. It is the responsibility of the caller to do this.
func (f *FixedBufferScanner) Close() {
	f.pool.Put(f)
}

var fixedBufferPoolsMap map[uint32]*sync.Pool
var fixedBufferPoolsMapMutex sync.RWMutex
var fixedBufferPoolsMapInit sync.Once

func newFixedBufferScannerPool(bufferSize uint32) *sync.Pool {
	return &sync.Pool{New: func() interface{} {
		return &FixedBufferScanner{bufferSize: bufferSize, buffer: make([]byte, bufferSize)}
	}}
}

func initFixedBufferPoolsMap() {
	fixedBufferPoolsMapMutex.Lock()
	fixedBufferPoolsMap = make(map[uint32]*sync.Pool)
	// pre-initialize the buffers that the milter library might request
	fixedBufferPoolsMap[1024*64-1] = newFixedBufferScannerPool(1024*64 - 1)
	fixedBufferPoolsMap[1024*256-1] = newFixedBufferScannerPool(1024*256 - 1)
	fixedBufferPoolsMap[1024*1024-1] = newFixedBufferScannerPool(1024*1024 - 1)
	fixedBufferPoolsMapMutex.Unlock()
}

// GetFixedBufferScanner returns a FixedBufferScanner of size bufferSize that is configured to read from r.
//
// It is the responsibility of the caller to close r.
//
// If the caller is done with the returned FixedBufferScanner its Close method should be called to release
// it to the shared pool of FixedBufferScanners.
func GetFixedBufferScanner(bufferSize uint32, r io.Reader) *FixedBufferScanner {
	fixedBufferPoolsMapInit.Do(initFixedBufferPoolsMap)
	// try with read lock first
	fixedBufferPoolsMapMutex.RLock()
	pool := fixedBufferPoolsMap[bufferSize]
	fixedBufferPoolsMapMutex.RUnlock()
	if pool == nil {
		// no luck, then get write lock
		fixedBufferPoolsMapMutex.Lock()
		// re-check the existence of pool
		if pool = fixedBufferPoolsMap[bufferSize]; pool == nil {
			// create pool in write lock
			pool = newFixedBufferScannerPool(bufferSize)
			fixedBufferPoolsMap[bufferSize] = pool
		}
		fixedBufferPoolsMapMutex.Unlock()
	}
	buffer := pool.Get().(*FixedBufferScanner)
	buffer.init(pool, r)
	return buffer
}
