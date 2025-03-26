package milterutil

import (
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
