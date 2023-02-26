package milter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/d--j/go-milter/internal/wire"
)

func TestRejectWithCodeAndReason(t *testing.T) {
	t.Parallel()
	tooBig := strings.Repeat("%%%%%%%%%%%%%%%%", 3000)
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
		{"Simple", args{400, "go away"}, "400 go away", false},
		{"Multi", args{400, "go away\r\nreally!"}, "400-go away\r\n400 really!", false},
		{"Trailing CRLF", args{400, "go away\r\nreally!\r\n"}, "400-go away\r\n400 really!", false},
		{"Empty", args{400, ""}, "400 ", false},
		{"Newline1", args{400, "\n"}, "400 ", false},
		{"Newline2", args{400, "\r"}, "400 ", false},
		{"Newline3", args{400, "\r\n"}, "400 ", false},
		{"Newline4", args{400, "\n\r"}, "400 ", false},
		{"%", args{400, "%"}, "400 %%", false},
		{"null-bytes", args{400, "bogus\x00reason"}, "", true},
		{"invalid-code1", args{200, ""}, "", true},
		{"invalid-code2", args{999, ""}, "", true},
		{"too-big", args{400, tooBig}, "", true},
		{"too-big", args{400, tooBig + tooBig}, "", true},
	}
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			response, err := RejectWithCodeAndReason(tt.args.smtpCode, tt.args.reason)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RejectWithCodeAndReason() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if response == nil {
				t.Fatalf("response <nil>")
			}
			if response.code != wire.Code(wire.ActReplyCode) {
				t.Fatalf("response.code got %c, want %c", response.code, wire.ActReplyCode)
			}
			got := string(response.data[0 : len(response.data)-1])
			if got != tt.want {
				t.Errorf("RejectWithCodeAndReason() got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCustomResponseDefaultResponse(t *testing.T) {
	tests := []struct {
		name         string
		r            *Response
		wantContinue bool
		wantMsg      *wire.Message
	}{
		{"RespContinue", RespContinue, true, &wire.Message{Code: wire.Code(wire.ActContinue)}},
		{"RespSkip", RespSkip, true, &wire.Message{Code: wire.Code(wire.ActSkip)}},
		{"RespAccept", RespAccept, false, &wire.Message{Code: wire.Code(wire.ActAccept)}},
		{"RespDiscard", RespDiscard, false, &wire.Message{Code: wire.Code(wire.ActDiscard)}},
		{"RespReject", RespReject, false, &wire.Message{Code: wire.Code(wire.ActReject)}},
		{"RespTempFail", RespTempFail, false, &wire.Message{Code: wire.Code(wire.ActTempFail)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotContinue := tt.r.Continue(); gotContinue != tt.wantContinue {
				t.Errorf("Continue() = %v, want %v", gotContinue, tt.wantContinue)
			}
			if gotResponse := tt.r.Response(); !reflect.DeepEqual(gotResponse, tt.wantMsg) {
				t.Errorf("Response() = %v, want %v", gotResponse, tt.wantMsg)
			}
		})
	}
}
