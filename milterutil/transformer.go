package milterutil

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

const cr = '\r'
const lf = '\n'

// CrLfToLfTransformer is a [transform.Transformer] that replaces all CR LF and single CR in src to LF in dst.
type CrLfToLfTransformer struct {
	prevCR bool
}

func (t *CrLfToLfTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nDst < len(dst) && nSrc < len(src) {
		c := src[nSrc]
		if c == lf {
			if t.prevCR {
				nSrc++
				t.prevCR = false
				continue
			}
		}
		t.prevCR = c == cr
		if t.prevCR {
			c = lf
		}
		dst[nDst] = c
		nDst++
		nSrc++
	}
	if nSrc < len(src) { // should never happen since we do not add data, but let's be safe
		err = transform.ErrShortDst
	}
	// if the last char in src is cr then there might be a lf coming
	if err == nil && !atEOF && len(src) > 0 && src[len(src)-1] == cr {
		err = transform.ErrShortSrc
		nSrc--
		nDst--
		return
	}
	return
}

func (t *CrLfToLfTransformer) Reset() {
	t.prevCR = false
}

var _ transform.Transformer = &CrLfToLfTransformer{}

// CrLfToLf is a helper that uses [CrLfToLfTransformer] to replace all line endings to only LF.
//
// postfix wants LF lines endings for header values. Using CRLF results in double CR sequences.
func CrLfToLf(s string) string {
	dst, _, err := transform.String(&CrLfToLfTransformer{}, s)
	if err != nil {
		panic(err)
	}
	return dst
}

// CrLfCanonicalizationTransformer is a [transform.Transformer] that replaces line endings in src with CR LF line endings in dst.
type CrLfCanonicalizationTransformer struct {
	prev byte
}

func (t *CrLfCanonicalizationTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nDst < len(dst) && nSrc < len(src) {
		c := src[nSrc]
		if c == lf {
			if t.prev != cr {
				if len(dst) <= nDst+1 {
					err = transform.ErrShortDst
					return
				}
				dst[nDst] = cr
				nDst++
			}
		} else if c == cr {
			if !atEOF && len(src) <= nSrc+1 {
				err = transform.ErrShortSrc
				return
			}
			if (atEOF && len(src) == nSrc+1) || src[nSrc+1] != lf {
				if len(dst) <= nDst+1 {
					err = transform.ErrShortDst
					return
				}
				dst[nDst] = c
				nDst++
				c = lf
			}
		}
		dst[nDst] = c
		nDst++
		nSrc++
		t.prev = c
	}
	if nSrc < len(src) {
		err = transform.ErrShortDst
	}
	return
}

func (t *CrLfCanonicalizationTransformer) Reset() {
	t.prev = 0
}

var _ transform.Transformer = &CrLfCanonicalizationTransformer{}

// DoublePercentTransformer is a [transform.Transformer] that replaces all % in src with %% in dst.
type DoublePercentTransformer struct {
	transform.NopResetter
}

func (t *DoublePercentTransformer) Transform(dst, src []byte, _ bool) (nDst, nSrc int, err error) {
	for nDst < len(dst) && nSrc < len(src) {
		c := src[nSrc]
		if c == '%' {
			if len(dst) <= nDst+1 {
				err = transform.ErrShortDst
				return
			}
			dst[nDst] = c
			nDst++
		}
		dst[nDst] = c
		nDst++
		nSrc++
	}
	if nSrc < len(src) {
		err = transform.ErrShortDst
	}
	return
}

var _ transform.Transformer = &DoublePercentTransformer{}

// SkipDoublePercentTransformer is a [transform.Transformer] that replaces all %% in src to % in dst.
// Single % signs are left as-is.
type SkipDoublePercentTransformer struct {
	prevPercent       bool
	prevDoublePercent bool
}

func (t *SkipDoublePercentTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nDst < len(dst) && nSrc < len(src) {
		c := src[nSrc]
		if c == '%' {
			if t.prevPercent && !t.prevDoublePercent {
				t.prevDoublePercent = true
				nSrc++
				continue
			}
		}
		t.prevPercent = c == '%'
		t.prevDoublePercent = false
		dst[nDst] = c
		nDst++
		nSrc++
	}
	if nSrc < len(src) { // should never happen since we do not add data, but let's be safe
		err = transform.ErrShortDst
	}
	// if the last char in src is a lonely % then there might be a % coming
	if err == nil && !atEOF && len(src) > 0 && t.prevPercent && !t.prevDoublePercent {
		err = transform.ErrShortSrc
		t.prevPercent = false
		nSrc--
		nDst--
		return
	}
	return
}

func (t *SkipDoublePercentTransformer) Reset() {
	t.prevPercent = false
	t.prevDoublePercent = false
}

var _ transform.Transformer = &SkipDoublePercentTransformer{}

// SMTPReplyTransformer is a [transform.Transformer] that reads src and produces a valid SMTP response (including multi-line handling)
//
// This transformer does not handle CR LF canonicalization, but it needs src to be properly encoded in this way.
//
// When you combine this Transformer in a [transform.Chain] it can only handle lines with a maximum of 128 bytes.
type SMTPReplyTransformer struct {
	Code uint16
	init bool
}

var errStartWithLF = errors.New("SMTP reply cannot start with LF")

func (t *SMTPReplyTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	if !t.init && (t.Code < 100 || t.Code > 599) {
		return 0, 0, fmt.Errorf("milter: %d is not a valid SMTP code", t.Code)
	}
	// special case: empty string
	if atEOF && !t.init && len(src) == 0 {
		if len(dst) <= nDst+4 {
			return 0, 0, transform.ErrShortDst
		}
		nDst += copy(dst[nDst:], fmt.Sprintf("%d ", t.Code))
		return
	}

	for nDst < len(dst) && nSrc < len(src) {
		c := src[nSrc]
		if !t.init || c == lf {
			if len(dst) <= nDst+5 {
				err = transform.ErrShortDst
				return
			}
			if !t.init && c == lf {
				err = errStartWithLF
				return
			}
			// determine if there is a newline following
			newline := false
			for peek := nSrc + 1; peek < len(src); peek++ {
				if src[peek] == lf {
					newline = true
					break
				}
			}
			// request more data when there might be more data, and we did not find a newline
			if !atEOF && !newline {
				err = transform.ErrShortSrc
				return
			}
			// insert \n before the SMTP code
			if t.init {
				dst[nDst] = c
				nDst++
				nSrc++
			}
			if newline {
				nDst += copy(dst[nDst:], fmt.Sprintf("%d-", t.Code))
			} else {
				nDst += copy(dst[nDst:], fmt.Sprintf("%d ", t.Code))
			}
			// first char is missing
			if !t.init {
				t.init = true
				dst[nDst] = c
				nDst++
				nSrc++
			}
		} else {
			dst[nDst] = c
			nDst++
			nSrc++
		}
	}
	if nSrc < len(src) {
		err = transform.ErrShortDst
	}
	return
}

func (t *SMTPReplyTransformer) Reset() {
	t.init = false
}

var _ transform.Transformer = &SMTPReplyTransformer{}

// DefaultMaximumLineLength is the maximum line length (in bytes) that will be used by [MaximumLineLengthTransformer]
// when its MaximumLength value is zero.
// The SMTP protocol theoretically allows up to 1000 bytes. We default to 950 bytes since some MTAs do forceful line
// breaks at lower limits (e.g. 980 bytes).
const DefaultMaximumLineLength = 950

var errWrongMaximumLineLength = errors.New("MaximumLength must be 4 or more")

// MaximumLineLengthTransformer is a [transform.Transformer] that splits src into lines of at most MaximumLength bytes.
//
// CR and LF are considered new line indicators. They do not count to the line length.
//
// This transformer can handle UTF-8 input.
// Because of this we actually start tying to split lines at MaximumLength - 3 bytes.
// This way we can assure that one line is never bigger than MaximumLength bytes.
type MaximumLineLengthTransformer struct {
	MaximumLength uint
	length        uint
}

func (t *MaximumLineLengthTransformer) Transform(dst, src []byte, _ bool) (nDst, nSrc int, err error) {
	if t.MaximumLength == 0 {
		t.MaximumLength = DefaultMaximumLineLength
	}
	if t.MaximumLength < utf8.UTFMax {
		return 0, 0, errWrongMaximumLineLength
	}

	for nDst < len(dst) && nSrc < len(src) {
		c := src[nSrc]
		isCrOfLf := c == cr || c == lf
		// break when we find a valid UTF8 rune start near the end of the line
		// or when we reach the maximum (then the string has invalid UTF-8 anyway)
		if !isCrOfLf && ((t.length > t.MaximumLength-utf8.UTFMax && utf8.RuneStart(c)) || (t.length >= t.MaximumLength)) {
			if len(dst) <= nDst+2 {
				err = transform.ErrShortDst
				return
			}
			nDst += copy(dst[nDst:], "\r\n")
			t.length = 0
		}
		dst[nDst] = c
		nDst++
		nSrc++
		if isCrOfLf {
			t.length = 0
		} else {
			t.length++
		}
	}
	if nSrc < len(src) {
		err = transform.ErrShortDst
	}
	return
}

func (t *MaximumLineLengthTransformer) Reset() {
	t.length = 0
}

var _ transform.Transformer = &MaximumLineLengthTransformer{}
