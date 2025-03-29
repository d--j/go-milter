package body

// ErrReader is an io.Reader that always returns an error.
type ErrReader struct {
	Err error
}

func (e ErrReader) Read(_ []byte) (n int, err error) {
	return 0, e.Err
}
