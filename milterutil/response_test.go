package milterutil

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestFormatResponse(t *testing.T) {
	type args struct {
		smtpCode uint16
		reason   string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"EmptyReason", args{400, ""}, "400 ", false},
		{"SimpleReason", args{400, "Test 1"}, "400 Test 1", false},
		{"FixedReason", args{400, "Test 1\u0000"}, "400 Test 1 ", false},
		{"TrimmedReason1", args{400, "\n\n\n"}, "400 ", false},
		{"TrimmedReason2", args{400, "Line 1\r\n"}, "400 Line 1", false},
		{"Multiline1", args{400, "Line 1\nLine 2"}, "400-Line 1\r\n400 Line 2", false},
		{"Multiline2", args{400, "Line 1\r\nLine 2"}, "400-Line 1\r\n400 Line 2", false},
		{"Multiline3", args{400, "4.0.0 Line 1\nLine 2"}, "400-4.0.0 Line 1\r\n400 4.0.0 Line 2", false},
		{"Multiline4", args{400, "5.0.0 Line 1\nLine 2"}, "400-5.0.0 Line 1\r\n400 Line 2", false},
		{"Multiline5", args{400, "\nLine 1\nLine 2"}, "400-\r\n400-Line 1\r\n400 Line 2", false},
		{"WrongCode1", args{99, ""}, "", true},
		{"WrongCode2", args{600, ""}, "", true},
		{"TooBigIn", args{250, strings.Repeat(" ", 64*1024*1024)}, "", true},
		{"TooBigOut", args{250, strings.Repeat("1\n", (64*1024*1024)/2-10)}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatResponse(tt.args.smtpCode, tt.args.reason)
			if (err != nil) != tt.wantErr {
				t.Errorf("FormatResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FormatResponse() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_minFormattedResponseLen(t *testing.T) {
	check := func(t *testing.T, reason string, wantExact bool) {
		t.Helper()
		got, err := FormatResponse(250, reason)
		if err != nil {
			t.Fatalf("FormatResponse(%q) error = %v", reason, err)
		}
		minLen := minFormattedResponseLen(strings.TrimRight(reason, "\r\n"))
		if minLen > len(got) {
			t.Fatalf("minFormattedResponseLen(%q) = %d, but FormatResponse produced %d bytes: %q", reason, minLen, len(got), got)
		}
		if wantExact && minLen != len(got) {
			t.Fatalf("minFormattedResponseLen(%q) = %d, want exactly %d: %q", reason, minLen, len(got), got)
		}
	}
	for _, reason := range []string{
		"", "a", "%", "%%", "\x00", "\r", "\n", "\r\n", "\n\r", "\r\r\n", "\n\n", "a\rb", "a\nb", "a\r\nb", "\na",
		"\r\na\n%\r\x00b\n\rc",
	} {
		check(t, reason, true)
	}
	// long lines and enhanced error codes make the output longer than the lower bound
	check(t, strings.Repeat("a", 3000), false)
	check(t, "2.0.0 line 1\nline 2\nline 3", false)

	// random inputs made of the bytes that change the output length
	rng := rand.New(rand.NewPCG(1, 2))
	alphabet := []string{"a", "%", "\x00", "\r", "\n", "2.0.0 ", strings.Repeat("x", 500)}
	for range 5000 {
		var b strings.Builder
		for n := rng.IntN(20); n > 0; n-- {
			b.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		check(t, b.String(), false)
	}
}

// TestFormatResponse_TooBigAfterTransform covers the final length check in [FormatResponse]:
// the input passes the up-front lower bound check but the repeated RFC 2034 enhanced error code
// makes the formatted response exceed [MaxResponseSize].
func TestFormatResponse_TooBigAfterTransform(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: formats ~64 MiB of data")
	}
	// every "\na" line becomes "250-2.0.0 a\r\n" (13 bytes) but the lower bound only accounts for 7 bytes
	lines := (MaxResponseSize - 11) / 7
	reason := "2.0.0 x" + strings.Repeat("\na", lines)
	if minLen := minFormattedResponseLen(reason); minLen > MaxResponseSize {
		t.Fatalf("test input does not pass the up-front check: %d > %d", minLen, MaxResponseSize)
	}
	_, err := FormatResponse(250, reason)
	if err == nil {
		t.Fatalf("FormatResponse() error = nil, want error")
	}
	if strings.Contains(err.Error(), "at least") {
		t.Fatalf("FormatResponse() error = %v, want error from the final length check", err)
	}
}
