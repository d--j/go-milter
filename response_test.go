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

func TestResponse_String(t *testing.T) {
	type fields struct {
		code wire.Code
		data []byte
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"continue", fields{wire.Code(wire.ActContinue), nil}, "response=continue"},
		{"accept", fields{wire.Code(wire.ActAccept), nil}, "response=accept"},
		{"discard", fields{wire.Code(wire.ActDiscard), nil}, "response=discard"},
		{"reject", fields{wire.Code(wire.ActReject), nil}, "response=reject"},
		{"temp_fail", fields{wire.Code(wire.ActTempFail), nil}, "response=temp_fail"},
		{"skip", fields{wire.Code(wire.ActSkip), nil}, "response=skip"},
		{"progress", fields{wire.Code(wire.ActProgress), nil}, "response=progress"},
		{"reply_code1", fields{wire.Code(wire.ActReplyCode), []byte("444 test\x00")}, "response=reply_code action=temp_fail code=444 reason=\"444 test\""},
		{"reply_code2", fields{wire.Code(wire.ActReplyCode), []byte("555 test\x00")}, "response=reply_code action=reject code=555 reason=\"555 test\""},
		{"reply_code3", fields{wire.Code(wire.ActReplyCode), []byte("continue\x00")}, "response=invalid code=121 data_len=9 data=\"continue\\x00\""},
		{"add_rcpt1", fields{wire.Code(wire.ActAddRcpt), []byte("<>\x00")}, "response=add_rcpt rcpt=\"<>\""},
		{"add_rcpt2", fields{wire.Code(wire.ActAddRcptPar), []byte("<>\x00A=B\x00")}, "response=add_rcpt rcpt=\"<>\" args=\"A=B\""},
		{"del_rcpt", fields{wire.Code(wire.ActDelRcpt), []byte("<>\x00A=B\x00")}, "response=del_rcpt rcpt=\"<>\""},
		{"quarantine", fields{wire.Code(wire.ActQuarantine), []byte("spam\x00")}, "response=quarantine reason=\"spam\""},
		{"replace_body", fields{wire.Code(wire.ActReplBody), []byte("1234")}, "response=replace_body len=4"},
		{"change_from1", fields{wire.Code(wire.ActChangeFrom), []byte("<>\x00")}, "response=change_from from=\"<>\""},
		{"change_from2", fields{wire.Code(wire.ActChangeFrom), []byte("<>\x00A=B\x00")}, "response=change_from from=\"<>\" args=\"A=B\""},
		{"add_header", fields{wire.Code(wire.ActAddHeader), []byte("X-Test\x00Test\x00")}, "response=add_header name=\"X-Test\" value=\"Test\""},
		{"change_header", fields{wire.Code(wire.ActChangeHeader), []byte("\x00\x00\x00\x01X-Test\x00Test\x00")}, "response=change_header name=\"X-Test\" value=\"Test\" index=1"},
		{"insert_header", fields{wire.Code(wire.ActInsertHeader), []byte("\x00\x00\x00\x01X-Test\x00Test\x00")}, "response=insert_header name=\"X-Test\" value=\"Test\" index=1"},
		{"garbage", fields{wire.Code(0), []byte("\x00\x00\x00\x00")}, "response=unknown code=0 data_len=4 data=\"\\x00\\x00\\x00\\x00\""},
		{"garbage-nil", fields{wire.Code(128), nil}, "response=unknown code=128 data_len=0 data=\"\""},
	}
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			r := &Response{
				code: tt.fields.code,
				data: tt.fields.data,
			}
			if got := r.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
