package milterutil

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

type transformerTestCase struct {
	inputs   []string
	expected string
}
type transformerTestCases []transformerTestCase

func doTransformation(transformer transform.Transformer, inputs []string) ([]byte, error) {
	r, w := io.Pipe()
	go func() {
		for _, s := range inputs {
			if _, err := w.Write([]byte(s)); err != nil {
				_ = w.CloseWithError(err)
				return
			}
		}
		_ = w.Close()
	}()
	tr := transform.NewReader(r, transformer)
	return io.ReadAll(tr)
}

func doTransformerTest(t *testing.T, getTransformer func() transform.Transformer, extraCheck func(*testing.T, transformerTestCase, string), testCases transformerTestCases) {
	runTestCase := func(t *testing.T, tt transformerTestCase, transformer transform.Transformer) {
		output, err := doTransformation(transformer, tt.inputs)
		if err != nil {
			t.Fatal(err)
		}
		if string(output) != tt.expected {
			t.Fatalf("expected %q, got %q", tt.expected, string(output))
		}
		output2, _, err := transform.String(transformer, strings.Join(tt.inputs, ""))
		if err != nil {
			t.Fatal(err)
		}
		if output2 != tt.expected {
			t.Fatalf("expected %q, got %q", tt.expected, output2)
		}
		if extraCheck != nil {
			extraCheck(t, tt, output2)
		}
	}
	for i, tt := range testCases {
		prettyName := fmt.Sprintf(":%q", tt.inputs)
		if len(prettyName) > 50 {
			prettyName = fmt.Sprintf(":(%d inputs with %d bytes total)", len(tt.inputs), len(strings.Join(tt.inputs, "")))
		}
		t.Run(fmt.Sprintf("%d%s", i, prettyName), func(t *testing.T) {
			ltt := tt
			t.Parallel()
			runTestCase(t, ltt, getTransformer())
		})
	}
	t.Run("Reset", func(t *testing.T) {
		t.Parallel()
		transformer := getTransformer()
		for _, tt := range testCases {
			runTestCase(t, tt, transformer)
		}
	})
}

func TestCrLfToLfTransformer(t *testing.T) {
	// transform.Transformer uses initial dst buffer size of 4096 bytes
	stuffing := strings.Repeat("1234567890", 4090/10)
	t.Parallel()
	doTransformerTest(t, func() transform.Transformer {
		return &CrLfToLfTransformer{}
	}, nil, transformerTestCases{
		{[]string{""}, ""},
		{[]string{"\n"}, "\n"},
		{[]string{"\r"}, "\n"},
		{[]string{"\r\n"}, "\n"},
		{[]string{"\r\r\n"}, "\n\n"},
		{[]string{"\r\n\r"}, "\n\n"},
		{[]string{"\r\n\r\n"}, "\n\n"},
		{[]string{"line1\r\nline2\r\n"}, "line1\nline2\n"},
		{[]string{"\r", "\n"}, "\n"},
		{[]string{"\r\r", "\n"}, "\n\n"},
		{[]string{stuffing + "123456\r", "\n"}, stuffing + "123456\n"},
		// regression https://github.com/d--j/go-milter/pull/20
		{[]string{"aaaaaaaaaaaaaaaaaaaaaaaa\r\naaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\nbbbbbbb"}, "aaaaaaaaaaaaaaaaaaaaaaaa\naaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nbbbbbbb"},
	})
}

func TestCrLfCanonicalizationTransformer(t *testing.T) {
	// transform.Transformer uses initial dst buffer size of 4096 bytes
	stuffing := strings.Repeat("1234567890", 4090/10)
	manyCR := strings.Repeat("\r", 4095)
	manCRLF := strings.Repeat("\r\n", 4095)
	t.Parallel()
	doTransformerTest(t, func() transform.Transformer {
		return &CrLfCanonicalizationTransformer{}
	}, nil, transformerTestCases{
		{[]string{""}, ""},
		{[]string{"\n"}, "\r\n"},
		{[]string{"", "\n"}, "\r\n"},
		{[]string{"\r"}, "\r\n"},
		{[]string{"", "\r"}, "\r\n"},
		{[]string{"\r\n"}, "\r\n"},
		{[]string{"\r\r\n"}, "\r\n\r\n"},
		{[]string{"\r\n\r"}, "\r\n\r\n"},
		{[]string{"\r\n\r\n"}, "\r\n\r\n"},
		{[]string{"line1\nline2\r\nline3\n"}, "line1\r\nline2\r\nline3\r\n"},
		{[]string{"\r", "\n"}, "\r\n"},
		{[]string{"\r\r", "\n"}, "\r\n\r\n"},
		{[]string{"\n\x00\n"}, "\r\n\x00\r\n"},
		{[]string{stuffing + "123456\r", "\n"}, stuffing + "123456\r\n"},
		{[]string{manyCR}, manCRLF},
	})
}

func TestDoublePercentTransformer(t *testing.T) {
	// transform.Transformer uses initial dst buffer size of 4096 bytes
	stuffing := strings.Repeat("1234567890", 4090/10)
	manyPercent := strings.Repeat("%", 4096)
	t.Parallel()
	doTransformerTest(t, func() transform.Transformer {
		return &DoublePercentTransformer{}
	}, nil, transformerTestCases{
		{[]string{""}, ""},
		{[]string{"%"}, "%%"},
		{[]string{" % "}, " %% "},
		{[]string{"%%"}, "%%%%"},
		{[]string{" ", "%"}, " %%"},
		{[]string{"%", "%"}, "%%%%"},
		{[]string{"%\x00%"}, "%%\x00%%"},
		{[]string{stuffing + "12345%", "%"}, stuffing + "12345%%%%"},
		{[]string{manyPercent}, manyPercent + manyPercent},
	})
}

func TestSkipDoublePercentTransformer(t *testing.T) {
	// transform.Transformer uses initial dst buffer size of 4096 bytes
	stuffing := strings.Repeat("1234567890", 4090/10)
	t.Parallel()
	doTransformerTest(t, func() transform.Transformer {
		return &SkipDoublePercentTransformer{}
	}, nil, transformerTestCases{
		{[]string{""}, ""},
		{[]string{"%"}, "%"},
		{[]string{" % "}, " % "},
		{[]string{"%%"}, "%"},
		{[]string{"%", "%"}, "%"},
		{[]string{"%", "%", "%"}, "%%"},
		{[]string{"%%\x00%%"}, "%\x00%"},
		{[]string{stuffing + "12345%", "%"}, stuffing + "12345%"},
	})
}

func TestSMTPReplyTransformer(t *testing.T) {
	// transform.Transformer uses initial dst buffer size of 4096 bytes
	manyLines := strings.Repeat("12\r\n", 786) + "12"                 // 3146 bytes
	expectedManyLines := strings.Repeat("499-12\r\n", 786) + "499 12" // 6294 bytes
	t.Parallel()
	doTransformerTest(t, func() transform.Transformer {
		return &SMTPReplyTransformer{Code: 499}
	}, nil, transformerTestCases{
		{[]string{""}, "499 "},
		{[]string{"", ""}, "499 "},
		{[]string{"4.3.999 testing"}, "499 4.3.999 testing"},
		{[]string{"line1\r\nline2"}, "499-line1\r\n499 line2"},
		{[]string{"line1\r\nline2\r\n"}, "499-line1\r\n499-line2\r\n499 "},
		{[]string{"line1\nline2"}, "499-line1\n499 line2"},
		{[]string{manyLines}, expectedManyLines},
		{[]string{"4.3.999 testing\nline 2"}, "499-4.3.999 testing\n499 4.3.999 line 2"},
		{[]string{"4.3.999 testing\r\nline 2"}, "499-4.3.999 testing\r\n499 4.3.999 line 2"},
		{[]string{"10.3.999 testing\r\nline 2"}, "499-10.3.999 testing\r\n499 line 2"},
		{[]string{"4.1234.999 testing\r\nline 2"}, "499-4.1234.999 testing\r\n499 line 2"},
		{[]string{"4.1.9999 testing\r\nline 2"}, "499-4.1.9999 testing\r\n499 line 2"},
		{[]string{"5.3.999 testing\r\nline 2"}, "499-5.3.999 testing\r\n499 line 2"},
		{[]string{"4.03.1 testing\r\nline 2"}, "499-4.03.1 testing\r\n499 line 2"},
		{[]string{"4.3.009 testing\r\nline 2"}, "499-4.3.009 testing\r\n499 line 2"},
		{[]string{"4.a.1 testing\r\nline 2"}, "499-4.a.1 testing\r\n499 line 2"},
		{[]string{"4.1.1a testing\r\nline 2"}, "499-4.1.1a testing\r\n499 line 2"},
	})
	t.Run("no LF at start", func(t *testing.T) {
		t.Parallel()
		output, err := doTransformation(&SMTPReplyTransformer{Code: 499}, []string{"\n"})
		if err == nil {
			t.Fatalf("expected err, got <nil> output = %q", output)
		}
	})
	t.Run("invalid code", func(t *testing.T) {
		t.Parallel()
		output, err := doTransformation(&SMTPReplyTransformer{Code: 9999}, []string{"\n"})
		if err == nil {
			t.Fatalf("expected err, got <nil> output = %q", output)
		}
	})
}

func TestMaximumLineLengthTransformer(t *testing.T) {
	t.Parallel()
	doTransformerTest(t, func() transform.Transformer {
		return &MaximumLineLengthTransformer{MaximumLength: 20}
	}, func(t *testing.T, testCase transformerTestCase, output string) {
		r := regexp.MustCompile("\r\n|\r|\n")
		lines := r.Split(output, -1)
		for _, line := range lines {
			if len(line) > 20 {
				t.Fatalf("output contained line with more than 20 bytes: %q", line)
			}
		}
	}, transformerTestCases{
		{[]string{""}, ""},
		{[]string{"", ""}, ""},
		{[]string{"12345678901234567890123456789012"}, "12345678901234567\r\n890123456789012"},
		{[]string{"1234567890123456789012345678901234567890"}, "12345678901234567\r\n89012345678901234\r\n567890"},
		{[]string{"12345678901234567890\r\n12345678901234567890"}, "12345678901234567\r\n890\r\n12345678901234567\r\n890"},
		{[]string{"12345678901234567\r89012345678901234567890"}, "12345678901234567\r89012345678901234\r\n567890"},
		{[]string{"12345678901234567890\n12345678901234567890"}, "12345678901234567\r\n890\n12345678901234567\r\n890"},
		{[]string{"12345678901234567890", "\r\n12345678901234567890"}, "12345678901234567\r\n890\r\n12345678901234567\r\n890"},
		{[]string{"\r", "\n", "\r", "\n"}, "\r\n\r\n"},
		{[]string{"🚀🚀🚀🚀🚀"}, "🚀🚀🚀🚀🚀"},
		{[]string{"🚀🚀🚀1🚀🚀"}, "🚀🚀🚀1🚀\r\n🚀"},
		{[]string{"🚀🚀🚀12🚀🚀"}, "🚀🚀🚀12🚀\r\n🚀"},
		{[]string{"🚀🚀🚀123🚀🚀"}, "🚀🚀🚀123🚀\r\n🚀"},
		{[]string{"🚀🚀🚀1234🚀🚀"}, "🚀🚀🚀1234🚀\r\n🚀"},
		{[]string{"🚀🚀🚀12345🚀🚀"}, "🚀🚀🚀12345\r\n🚀🚀"},
	})
	t.Run("default line length", func(t *testing.T) {
		t.Parallel()
		line := strings.Repeat(".", DefaultMaximumLineLength-utf8.UTFMax+1)
		output, err := doTransformation(&MaximumLineLengthTransformer{}, []string{line + line})
		if err != nil {
			t.Fatalf("not expected err, got %s", err)
		}
		expected := line + "\r\n" + line
		if string(output) != expected {
			t.Fatalf("expected %q, got %q", expected, string(output))
		}
	})
	t.Run("enforce minimum", func(t *testing.T) {
		t.Parallel()
		_, err := doTransformation(&MaximumLineLengthTransformer{MaximumLength: 1}, []string{""})
		if !errors.Is(err, errWrongMaximumLineLength) {
			t.Fatalf("err got %s, expected %s", err, errWrongMaximumLineLength)
		}
	})
	t.Run("work with minimum", func(t *testing.T) {
		t.Parallel()
		output, err := doTransformation(&MaximumLineLengthTransformer{MaximumLength: 4}, []string{"1234"})
		if err != nil {
			t.Fatalf("not expected err, got %s", err)
		}
		expected := "1\r\n2\r\n3\r\n4"
		if string(output) != expected {
			t.Fatalf("expected %q, got %q", expected, string(output))
		}
	})
}

func TestCrLfToLf(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{"empty", "", ""},
		{"simple", "\r\n", "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CrLfToLf(tt.arg); got != tt.want {
				t.Errorf("CrLfToLf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func FuzzCrLfToLfTransformer_Transform(f *testing.F) {
	f.Add([]byte("\r\n"), []byte(""), true)
	f.Add([]byte("\r"), []byte("\n"), true)
	f.Add([]byte("one\r\ntwo"), []byte(""), true)
	f.Add([]byte("\r"), []byte(""), true)
	f.Add([]byte("one\rtwo"), []byte(""), true)
	f.Add([]byte("\n"), []byte(""), true)
	f.Add([]byte("one\ntwo"), []byte(""), true)
	f.Add([]byte("\r\r\n"), []byte(""), true)
	f.Add([]byte("\r\r"), []byte("\n"), true)
	f.Fuzz(func(t *testing.T, input1 []byte, input2 []byte, writeEmpty bool) {
		r, w := io.Pipe()
		go func() {
			if len(input1) > 0 || writeEmpty {
				if _, err := w.Write(input1); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			if len(input2) > 0 || writeEmpty {
				if _, err := w.Write(input2); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			_ = w.Close()
		}()
		output, err := io.ReadAll(transform.NewReader(r, &CrLfToLfTransformer{}))
		if err != nil {
			t.Fatal(err)
		}
		if len(output) > len(input1)+len(input2) {
			t.Fatalf("output bigger than input %d > %d", len(output), len(input1)+len(input2))
		}
		if bytes.Contains(output, []byte("\r\n")) {
			t.Fatal("output contains \\r\\n")
		}
	})
}

func FuzzCrLfCanonicalizationTransformer_Transform(f *testing.F) {
	lineEndingRegexp := regexp.MustCompile("\r\n|\n\r|\r|\n")
	f.Add([]byte("\r\n"), []byte(""), true)
	f.Add([]byte("\r"), []byte("\n"), true)
	f.Add([]byte("one\r\ntwo"), []byte(""), true)
	f.Add([]byte("\r"), []byte(""), true)
	f.Add([]byte("one\rtwo"), []byte(""), true)
	f.Add([]byte("\n"), []byte(""), true)
	f.Add([]byte("one\ntwo"), []byte(""), true)
	f.Add([]byte("\r\r\n"), []byte(""), true)
	f.Add([]byte("\r\r"), []byte("\n"), true)
	f.Fuzz(func(t *testing.T, input1 []byte, input2 []byte, writeEmpty bool) {
		r, w := io.Pipe()
		go func() {
			if len(input1) > 0 || writeEmpty {
				if _, err := w.Write(input1); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			if len(input2) > 0 || writeEmpty {
				if _, err := w.Write(input2); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			_ = w.Close()
		}()
		output, err := io.ReadAll(transform.NewReader(r, &CrLfCanonicalizationTransformer{}))
		if err != nil {
			t.Fatal(err)
		}
		if len(output) < len(input1)+len(input2) {
			t.Fatalf("output smaller than input %d < %d", len(output), len(input1)+len(input2))
		}
		lineEndings := lineEndingRegexp.FindAll(output, -1)
		for _, ending := range lineEndings {
			if !bytes.Equal(ending, []byte("\r\n")) {
				t.Fatalf("output contained wrong line ending: %q", ending)
			}
		}
	})
}

func FuzzMaximumLineLengthTransformer_Transform(f *testing.F) {
	lineEndingRegexp := regexp.MustCompile("\r\n|\n\r|\r|\n")
	f.Add(uint(20), []byte("\r\n"), []byte(""), true)
	f.Add(uint(4), []byte("\r"), []byte("\n"), true)
	f.Add(uint(20), []byte("one\r\ntwo"), []byte(""), true)
	f.Add(uint(20), []byte("\r"), []byte(""), true)
	f.Add(uint(20), []byte("one\rtwo"), []byte(""), true)
	f.Add(uint(20), []byte("\n"), []byte(""), true)
	f.Add(uint(20), []byte("one\ntwo"), []byte(""), true)
	f.Add(uint(20), []byte("\r\r\n"), []byte(""), true)
	f.Add(uint(20), []byte("\r\r"), []byte("\n"), true)
	f.Fuzz(func(t *testing.T, maxLineLength uint, input1 []byte, input2 []byte, writeEmpty bool) {
		if maxLineLength < 4 {
			return
		}
		r, w := io.Pipe()
		go func() {
			if len(input1) > 0 || writeEmpty {
				if _, err := w.Write(input1); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			if len(input2) > 0 || writeEmpty {
				if _, err := w.Write(input2); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			_ = w.Close()
		}()
		output, err := io.ReadAll(transform.NewReader(r, &MaximumLineLengthTransformer{MaximumLength: maxLineLength}))
		if err != nil {
			t.Fatal(err)
		}
		if len(output) < len(input1)+len(input2) {
			t.Fatalf("output smaller than input %d < %d", len(output), len(input1)+len(input2))
		}
		lines := lineEndingRegexp.Split(string(output), -1)
		for _, line := range lines {
			if len(line) > int(maxLineLength) {
				t.Fatalf("output contained line with more than %d bytes: %q", maxLineLength, line)
			}
		}
		if utf8.Valid(append(input1, input2...)) && !utf8.Valid(output) {
			t.Fatal("input is valid UTF-8 but output is not")
		}
	})
}

func FuzzSkipDoublePercentTransformer_Transform(f *testing.F) {
	f.Add([]byte("%"), []byte("%"), true)
	f.Add([]byte("%%"), []byte(""), true)
	f.Add([]byte(""), []byte("%"), true)
	f.Add([]byte(""), []byte("%%"), true)
	f.Fuzz(func(t *testing.T, input1 []byte, input2 []byte, writeEmpty bool) {
		r, w := io.Pipe()
		go func() {
			if len(input1) > 0 || writeEmpty {
				if _, err := w.Write(input1); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			if len(input2) > 0 || writeEmpty {
				if _, err := w.Write(input2); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			_ = w.Close()
		}()
		output, err := io.ReadAll(transform.NewReader(r, &SkipDoublePercentTransformer{}))
		if err != nil {
			t.Fatal(err)
		}
		if len(output) > len(input1)+len(input2) {
			t.Fatalf("output bigger than input %d > %d", len(output), len(input1)+len(input2))
		}
		if bytes.Contains(output, []byte("%%")) {
			t.Fatal("output contains %%")
		}
	})
}

func FuzzDoublePercentTransformer_Transform(f *testing.F) {
	singlePercentRegexp := regexp.MustCompile("[^%]%|%[^%]")
	f.Add([]byte("%"), []byte("%"), true)
	f.Add([]byte("%%"), []byte(""), true)
	f.Add([]byte(""), []byte("%"), true)
	f.Add([]byte(""), []byte("%%"), true)
	f.Fuzz(func(t *testing.T, input1 []byte, input2 []byte, writeEmpty bool) {
		r, w := io.Pipe()
		go func() {
			if len(input1) > 0 || writeEmpty {
				if _, err := w.Write(input1); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			if len(input2) > 0 || writeEmpty {
				if _, err := w.Write(input2); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			_ = w.Close()
		}()
		output, err := io.ReadAll(transform.NewReader(r, &DoublePercentTransformer{}))
		if err != nil {
			t.Fatal(err)
		}
		if len(output) < len(input1)+len(input2) {
			t.Fatalf("output smaller than input %d < %d", len(output), len(input1)+len(input2))
		}
		if singlePercentRegexp.Match(output) {
			t.Fatal("output contains single %")
		}
	})
}

func FuzzSMTPReplyTransformer_Transform(f *testing.F) {
	f.Add([]byte("\r\n"), []byte(""), true)
	f.Add([]byte("\r"), []byte("\n"), true)
	f.Add([]byte("one\r\ntwo"), []byte(""), true)
	f.Add([]byte("\r"), []byte(""), true)
	f.Add([]byte("one\rtwo"), []byte(""), true)
	f.Add([]byte("\n"), []byte(""), true)
	f.Add([]byte("one\ntwo"), []byte(""), true)
	f.Add([]byte("\r\r\n"), []byte(""), true)
	f.Add([]byte("\r\r"), []byte("\n"), true)
	f.Add([]byte("a long line"), []byte("a long line"), true)
	f.Fuzz(func(t *testing.T, input1 []byte, input2 []byte, writeEmpty bool) {
		r, w := io.Pipe()
		lw := transform.NewWriter(w, &MaximumLineLengthTransformer{MaximumLength: 920})
		go func() {
			if len(input1) > 0 || writeEmpty {
				if _, err := lw.Write(input1); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			if len(input2) > 0 || writeEmpty {
				if _, err := lw.Write(input2); err != nil {
					_ = w.CloseWithError(err)
					return
				}
			}
			if err := lw.Close(); err != nil {
				_ = w.CloseWithError(err)
			} else {
				_ = w.Close()
			}
		}()
		output, err := io.ReadAll(transform.NewReader(r, &SMTPReplyTransformer{Code: 300}))
		if err != nil {
			if err == errStartWithLF {
				return
			}
			t.Fatal(err)
		}
		if len(output) < len(input1)+len(input2) {
			t.Fatalf("output smaller than input %d < %d", len(output), len(input1)+len(input2))
		}
		or := textproto.NewReader(bufio.NewReader(bytes.NewReader(output)))
		if _, _, err := or.ReadResponse(300); err != nil {
			t.Fatalf("not valid SMTP response: %q", output)
		}
	})
}
