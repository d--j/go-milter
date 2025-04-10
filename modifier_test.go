package milter

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/d--j/go-milter/internal/wire"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestAction_StopProcessing(t *testing.T) {
	tests := []struct {
		name       string
		actionType ActionType
		want       bool
	}{
		{"empty", ActionType(0), false},
		{"ActionAccept", ActionAccept, false},
		{"ActionContinue", ActionContinue, false},
		{"ActionDiscard", ActionDiscard, false},
		{"ActionReject", ActionReject, true},
		{"ActionTempFail", ActionTempFail, true},
		{"ActionSkip", ActionSkip, false},
		{"ActionRejectWithCode", ActionRejectWithCode, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Action{
				Type: tt.actionType,
			}
			if got := a.StopProcessing(); got != tt.want {
				t.Errorf("StopProcessing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAction_String(t *testing.T) {
	type fields struct {
		Type      ActionType
		SMTPCode  uint16
		SMTPReply string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"ActionAccept", fields{ActionAccept, 0, ""}, "Accept"},
		{"ActionContinue", fields{ActionContinue, 0, ""}, "Continue"},
		{"ActionDiscard", fields{ActionDiscard, 0, ""}, "Discard"},
		{"ActionReject", fields{ActionReject, 500, "reject"}, "Reject 500 \"reject\""},
		{"ActionTempFail", fields{ActionTempFail, 400, "temp"}, "TempFail 400 \"temp\""},
		{"ActionSkip", fields{ActionSkip, 0, ""}, "Skip"},
		{"ActionRejectWithCode", fields{ActionRejectWithCode, 444, "temp"}, "RejectWithCode 444 \"temp\""},
		{"unknown", fields{ActionType(99), 0, ""}, "Unknown action 99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Action{
				Type:      tt.fields.Type,
				SMTPCode:  tt.fields.SMTPCode,
				SMTPReply: tt.fields.SMTPReply,
			}
			if got := a.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseAction(t *testing.T) {
	tests := []struct {
		name    string
		msg     *wire.Message
		want    *Action
		wantErr bool
	}{
		{"accept", &wire.Message{Code: wire.Code(wire.ActAccept)}, &Action{Type: ActionAccept, SMTPCode: 250, SMTPReply: "250 accept"}, false},
		{"continue", &wire.Message{Code: wire.Code(wire.ActContinue)}, &Action{Type: ActionContinue, SMTPCode: 250, SMTPReply: "250 accept"}, false},
		{"discard", &wire.Message{Code: wire.Code(wire.ActDiscard)}, &Action{Type: ActionDiscard, SMTPCode: 250, SMTPReply: "250 accept"}, false},
		{"reject", &wire.Message{Code: wire.Code(wire.ActReject)}, &Action{Type: ActionReject, SMTPCode: 550, SMTPReply: "550 5.7.1 Command rejected"}, false},
		{"temp-fail", &wire.Message{Code: wire.Code(wire.ActTempFail)}, &Action{Type: ActionTempFail, SMTPCode: 451, SMTPReply: "451 4.7.1 Service unavailable - try again later"}, false},
		{"skip", &wire.Message{Code: wire.Code(wire.ActSkip)}, &Action{Type: ActionSkip, SMTPCode: 250, SMTPReply: "250 accept"}, false},
		{"repl-code", &wire.Message{Code: wire.Code(wire.ActReplyCode), Data: []byte("550 5.7.1 Reject\u0000")}, &Action{Type: ActionRejectWithCode, SMTPCode: 550, SMTPReply: "550 5.7.1 Reject"}, false},
		{"repl-code-trim", &wire.Message{Code: wire.Code(wire.ActReplyCode), Data: []byte("550 5.7.1 Reject\r\n\u0000")}, &Action{Type: ActionRejectWithCode, SMTPCode: 550, SMTPReply: "550 5.7.1 Reject"}, false},
		{"repl-code-double-nul", &wire.Message{Code: wire.Code(wire.ActReplyCode), Data: []byte("550 5.7.1 Reject\u0000stuff\u0000")}, &Action{Type: ActionRejectWithCode, SMTPCode: 550, SMTPReply: "550 5.7.1 Reject"}, false},
		{"repl-code-multiline", &wire.Message{Code: wire.Code(wire.ActReplyCode), Data: []byte("550-5.7.1 Reject\r\n550-5.7.1 this\r\n550 5.7.1 message\u0000")}, &Action{Type: ActionRejectWithCode, SMTPCode: 550, SMTPReply: "550-5.7.1 Reject\r\n550-5.7.1 this\r\n550 5.7.1 message"}, false},
		{"repl-code-multiline-trim", &wire.Message{Code: wire.Code(wire.ActReplyCode), Data: []byte("550-5.7.1 Reject\r\n550-5.7.1 this\r\n550 5.7.1 message\r\n\u0000")}, &Action{Type: ActionRejectWithCode, SMTPCode: 550, SMTPReply: "550-5.7.1 Reject\r\n550-5.7.1 this\r\n550 5.7.1 message"}, false},
		{"repl-code-err", &wire.Message{Code: wire.Code(wire.ActReplyCode)}, nil, true},
		{"repl-code-err-wo-nul", &wire.Message{Code: wire.Code(wire.ActReplyCode), Data: []byte("550 5.7.1 Reject")}, nil, true},
		{"repl-code-err-invalid-code", &wire.Message{Code: wire.Code(wire.ActReplyCode), Data: []byte("250 Accept\u0000")}, nil, true},
		{"unknown", &wire.Message{Code: wire.Code('?'), Data: []byte("550 Reject\u0000")}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAction(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAction() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAction() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModifyAction_String(t *testing.T) {
	type fields struct {
		Type        ModifyActionType
		Rcpt        string
		RcptArgs    string
		From        string
		FromArgs    string
		Body        []byte
		HeaderIndex uint32
		HeaderName  string
		HeaderValue string
		Reason      string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"ActionAddRcpt", fields{Type: ActionAddRcpt, Rcpt: "test", RcptArgs: "A=B"}, "AddRcpt \"test\" \"A=B\""},
		{"ActionDelRcpt", fields{Type: ActionDelRcpt, Rcpt: "test"}, "DelRcpt \"test\""},
		{"ActionQuarantine", fields{Type: ActionQuarantine, Reason: "test"}, "Quarantine \"test\""},
		{"ActionReplaceBody", fields{Type: ActionReplaceBody}, "ReplaceBody len(body) = 0 sha1(body) = da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{"ActionChangeFrom", fields{Type: ActionChangeFrom, From: "test", FromArgs: "A=B"}, "ChangeFrom \"test\" \"A=B\""},
		{"ActionAddHeader", fields{Type: ActionAddHeader, HeaderName: "test", HeaderValue: " value"}, "AddHeader \"test\" \" value\""},
		{"ActionChangeHeader", fields{Type: ActionChangeHeader, HeaderIndex: 1, HeaderName: "test", HeaderValue: " value"}, "ChangeHeader 1 \"test\" \" value\""},
		{"ActionInsertHeader", fields{Type: ActionInsertHeader, HeaderIndex: 1, HeaderName: "test", HeaderValue: " value"}, "InsertHeader 1 \"test\" \" value\""},
		{"unknown", fields{Type: 99}, "Unknown modify action 99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := ModifyAction{
				Type:        tt.fields.Type,
				Rcpt:        tt.fields.Rcpt,
				RcptArgs:    tt.fields.RcptArgs,
				From:        tt.fields.From,
				FromArgs:    tt.fields.FromArgs,
				Body:        tt.fields.Body,
				HeaderIndex: tt.fields.HeaderIndex,
				HeaderName:  tt.fields.HeaderName,
				HeaderValue: tt.fields.HeaderValue,
				Reason:      tt.fields.Reason,
			}
			if got := ma.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseModifyAct(t *testing.T) {
	tests := []struct {
		name    string
		msg     *wire.Message
		want    *ModifyAction
		wantErr bool
	}{
		{"ActionAddRcpt", &wire.Message{Code: wire.Code(wire.ActAddRcpt), Data: []byte("<>\u0000")}, &ModifyAction{Type: ActionAddRcpt, Rcpt: "<>"}, false},
		{"ActionAddRcpt-err1", &wire.Message{Code: wire.Code(wire.ActAddRcpt), Data: []byte("<>\u0000\u0000")}, nil, true},
		{"ActionAddRcpt-err2", &wire.Message{Code: wire.Code(wire.ActAddRcpt), Data: []byte("<>")}, nil, true},
		{"ActAddRcptPar1", &wire.Message{Code: wire.Code(wire.ActAddRcptPar), Data: []byte("<>\u0000")}, &ModifyAction{Type: ActionAddRcpt, Rcpt: "<>"}, false},
		{"ActAddRcptPar2", &wire.Message{Code: wire.Code(wire.ActAddRcptPar), Data: []byte("<>\u0000A=B\u0000")}, &ModifyAction{Type: ActionAddRcpt, Rcpt: "<>", RcptArgs: "A=B"}, false},
		{"ActAddRcptPar-err1", &wire.Message{Code: wire.Code(wire.ActAddRcptPar), Data: []byte("<>\u0000\u0000\u0000")}, nil, true},
		{"ActAddRcptPar-err2", &wire.Message{Code: wire.Code(wire.ActAddRcptPar), Data: []byte("<>")}, nil, true},
		{"ActChangeFrom1", &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>\u0000")}, &ModifyAction{Type: ActionChangeFrom, From: "<>"}, false},
		{"ActChangeFrom2", &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>\u0000A=B\u0000")}, &ModifyAction{Type: ActionChangeFrom, From: "<>", FromArgs: "A=B"}, false},
		{"ActChangeFrom-err1", &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>\u0000\u0000\u0000")}, nil, true},
		{"ActChangeFrom-err2", &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>")}, nil, true},
		{"ActDelRcpt", &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("<>\u0000")}, &ModifyAction{Type: ActionDelRcpt, Rcpt: "<>"}, false},
		{"ActDelRcpt-double-nul", &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("<>\u0000stuff\u0000")}, &ModifyAction{Type: ActionDelRcpt, Rcpt: "<>"}, false},
		{"ActDelRcpt-err", &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("<>")}, nil, true},
		{"ActQuarantine", &wire.Message{Code: wire.Code(wire.ActQuarantine), Data: []byte("reason\u0000")}, &ModifyAction{Type: ActionQuarantine, Reason: "reason"}, false},
		{"ActQuarantine-double-nul", &wire.Message{Code: wire.Code(wire.ActQuarantine), Data: []byte("reason\u0000stuff\u0000")}, &ModifyAction{Type: ActionQuarantine, Reason: "reason"}, false},
		{"ActQuarantine-no-nul", &wire.Message{Code: wire.Code(wire.ActQuarantine), Data: []byte("reason")}, nil, true},
		{"ActQuarantine-err", &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("reason")}, nil, true},
		{"ActReplBody", &wire.Message{Code: wire.Code(wire.ActReplBody), Data: []byte("data")}, &ModifyAction{Type: ActionReplaceBody, Body: []byte("data")}, false},
		{"ActChangeHeader", &wire.Message{Code: wire.Code(wire.ActChangeHeader), Data: []byte{0, 0, 0, 1, 'S', 'u', 'b', 'j', 'e', 'c', 't', 0, 'v', 'a', 'l', 'u', 'e', 0}}, &ModifyAction{Type: ActionChangeHeader, HeaderIndex: 1, HeaderName: "Subject", HeaderValue: "value"}, false},
		{"ActChangeHeader-sendmail", &wire.Message{Code: wire.Code(wire.ActChangeHeader), Data: []byte{0, 0, 0, 0, 'S', 'u', 'b', 'j', 'e', 'c', 't', 0, 'v', 'a', 'l', 'u', 'e', 0}}, &ModifyAction{Type: ActionChangeHeader, HeaderIndex: 1, HeaderName: "Subject", HeaderValue: "value"}, false},
		{"ActInsertHeader", &wire.Message{Code: wire.Code(wire.ActInsertHeader), Data: []byte{0, 0, 0, 1, 'S', 'u', 'b', 'j', 'e', 'c', 't', 0, 'v', 'a', 'l', 'u', 'e', 0}}, &ModifyAction{Type: ActionInsertHeader, HeaderIndex: 1, HeaderName: "Subject", HeaderValue: "value"}, false},
		{"ActAddHeader", &wire.Message{Code: wire.Code(wire.ActAddHeader), Data: []byte{'S', 'u', 'b', 'j', 'e', 'c', 't', 0, 'v', 'a', 'l', 'u', 'e', 0}}, &ModifyAction{Type: ActionAddHeader, HeaderName: "Subject", HeaderValue: "value"}, false},
		{"ActChangeHeader-err", &wire.Message{Code: wire.Code(wire.ActChangeHeader), Data: []byte{0, 0, 0, 1, 'S', 'u', 'b', 'j', 'e', 'c', 't', 0, 'v', 'a', 'l', 'u', 'e'}}, nil, true},
		{"ActInsertHeader-err", &wire.Message{Code: wire.Code(wire.ActInsertHeader), Data: []byte{0, 0, 0, 1, 'S', 'u', 'b', 'j', 'e', 'c', 't', 0, 'v', 'a', 'l', 'u', 'e'}}, nil, true},
		{"ActAddHeader-err", &wire.Message{Code: wire.Code(wire.ActInsertHeader), Data: []byte{'S', 'u', 'b', 'j', 'e', 'c', 't', 0, 'v', 'a', 'l', 'u', 'e'}}, nil, true},
		{"unknown", &wire.Message{Code: wire.Code('?'), Data: []byte("\u0000")}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseModifyAct(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseModifyAct() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseModifyAct() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddAngle(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{"empty", "", "<>"},
		{"no angle", "test", "<test>"},
		{"with angle", "<test>", "<test>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AddAngle(tt.arg); got != tt.want {
				t.Errorf("AddAngle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveAngle(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{"empty", "", ""},
		{"no angle", "test", "test"},
		{"with angle", "<test>", "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveAngle(tt.arg); got != tt.want {
				t.Errorf("RemoveAngle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_AddHeader(t *testing.T) {
	type fields struct {
		state   modifierState
		actions OptAction
	}
	type args struct {
		name  string
		value string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"Subject", "Test"}, &wire.Message{Code: wire.Code(wire.ActAddHeader), Data: []byte("Subject\000Test\000")}, false},
		{"read-only", fields{modifierStateProgressOnly, AllClientSupportedActionMasks}, args{"Subject", "Test"}, nil, true},
		{"not-negotiated", fields{modifierStateReadWrite, 0}, args{"Subject", "Test"}, nil, true},
		{"invalid-name-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"Subject:", "Test"}, nil, true},
		{"invalid-name-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{" Subject", "Test"}, nil, true},
		{"invalid-name-3", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"Subj\u0000ect", "Test"}, nil, true},
		{"invalid-name-4", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"Subject💣", "Test"}, nil, true},
		{"invalid-name-5", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", "Test"}, nil, true},
		{"fixup-value-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"Subject", "Test\r\n Line2"}, &wire.Message{Code: wire.Code(wire.ActAddHeader), Data: []byte("Subject\000Test\n Line2\000")}, false},
		{"fixup-value-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"Subject", "Test\u0000ing"}, &wire.Message{Code: wire.Code(wire.ActAddHeader), Data: []byte("Subject\000Test ing\000")}, false},
		{"err-too-long", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"Subject", strings.Repeat(".", int(DataSize64K))}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				state:       tt.fields.state,
				writePacket: writePacket,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.AddHeader(tt.args.name, tt.args.value); (err != nil) != tt.wantErr {
				t.Errorf("AddHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_AddRecipient(t *testing.T) {
	type fields struct {
		version uint32
		state   modifierState
		actions OptAction
	}
	type args struct {
		r         string
		esmtpArgs string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", ""}, &wire.Message{Code: wire.Code(wire.ActAddRcpt), Data: []byte("<>\u0000")}, false},
		{"allowed-args", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", "A=B"}, &wire.Message{Code: wire.Code(wire.ActAddRcptPar), Data: []byte("<>\u0000A=B\u0000")}, false},
		{"allowed-args-2", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks & ^OptAddRcpt}, args{"", ""}, &wire.Message{Code: wire.Code(wire.ActAddRcptPar), Data: []byte("<>\u0000\u0000")}, false},
		{"read-only", fields{6, modifierStateProgressOnly, AllClientSupportedActionMasks}, args{"", ""}, nil, true},
		{"not-negotiated", fields{6, modifierStateReadWrite, 0}, args{"", ""}, nil, true},
		{"fixup-1", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<>", ""}, &wire.Message{Code: wire.Code(wire.ActAddRcpt), Data: []byte("<>\u0000")}, false},
		{"fixup-2", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<\u0000>", ""}, &wire.Message{Code: wire.Code(wire.ActAddRcpt), Data: []byte("< >\u0000")}, false},
		{"fixup-3", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<>", "\u0000"}, &wire.Message{Code: wire.Code(wire.ActAddRcptPar), Data: []byte("<>\u0000 \u0000")}, false},
		{"allowed-v2", fields{2, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", ""}, &wire.Message{Code: wire.Code(wire.ActAddRcpt), Data: []byte("<>\u0000")}, false},
		{"version-mismatch", fields{2, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", "A=B"}, nil, true},
		{"args-not-allowed", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks & ^OptAddRcptWithArgs}, args{"", "A=B"}, nil, true},
		{"err-too-long", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{strings.Repeat(".", int(DataSize64K)), ""}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				version:     tt.fields.version,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.AddRecipient(tt.args.r, tt.args.esmtpArgs); (err != nil) != tt.wantErr {
				t.Errorf("AddRecipient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddRecipient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_ChangeFrom(t *testing.T) {
	type fields struct {
		version uint32
		state   modifierState
		actions OptAction
	}
	type args struct {
		value     string
		esmtpArgs string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", ""}, &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>\u0000")}, false},
		{"allowed-args", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", "A=B"}, &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>\u0000A=B\u0000")}, false},
		{"read-only", fields{6, modifierStateProgressOnly, AllClientSupportedActionMasks}, args{"", ""}, nil, true},
		{"not-negotiated", fields{6, modifierStateReadWrite, 0}, args{"", ""}, nil, true},
		{"fixup-1", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<>", ""}, &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>\u0000")}, false},
		{"fixup-2", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<\u0000>", ""}, &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("< >\u0000")}, false},
		{"fixup-3", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<>", "\u0000"}, &wire.Message{Code: wire.Code(wire.ActChangeFrom), Data: []byte("<>\u0000 \u0000")}, false},
		{"version-mismatch", fields{2, modifierStateReadWrite, AllClientSupportedActionMasks}, args{"", ""}, nil, true},
		{"err-too-long", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, args{strings.Repeat(".", int(DataSize64K)), ""}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				version:     tt.fields.version,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.ChangeFrom(tt.args.value, tt.args.esmtpArgs); (err != nil) != tt.wantErr {
				t.Errorf("ChangeFrom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChangeFrom() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_ChangeHeader(t *testing.T) {
	type fields struct {
		state   modifierState
		actions OptAction
	}
	type args struct {
		index int
		name  string
		value string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", "Test"}, &wire.Message{Code: wire.Code(wire.ActChangeHeader), Data: []byte("\u0000\u0000\u0000\u0001Subject\000Test\000")}, false},
		{"read-only", fields{modifierStateProgressOnly, AllClientSupportedActionMasks}, args{1, "Subject", "Test"}, nil, true},
		{"not-negotiated", fields{modifierStateReadWrite, 0}, args{1, "Subject", "Test"}, nil, true},
		{"invalid-index-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{-1, "Subject:", "Test"}, nil, true},
		{"invalid-index-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{math.MaxUint32 + 1, "Subject:", "Test"}, nil, true},
		{"invalid-name-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject:", "Test"}, nil, true},
		{"invalid-name-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, " Subject", "Test"}, nil, true},
		{"invalid-name-3", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subj\u0000ect", "Test"}, nil, true},
		{"invalid-name-4", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject💣", "Test"}, nil, true},
		{"invalid-name-5", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "", "Test"}, nil, true},
		{"fixup-value-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", "Test\r\n Line2"}, &wire.Message{Code: wire.Code(wire.ActChangeHeader), Data: []byte("\u0000\u0000\u0000\u0001Subject\000Test\n Line2\000")}, false},
		{"fixup-value-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", "Test\u0000ing"}, &wire.Message{Code: wire.Code(wire.ActChangeHeader), Data: []byte("\u0000\u0000\u0000\u0001Subject\000Test ing\000")}, false},
		{"err-too-long", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", strings.Repeat(".", int(DataSize64K))}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.ChangeHeader(tt.args.index, tt.args.name, tt.args.value); (err != nil) != tt.wantErr {
				t.Errorf("ChangeHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChangeHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_DeleteRecipient(t *testing.T) {
	type fields struct {
		state   modifierState
		actions OptAction
	}
	type args struct {
		r string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{""}, &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("<>\u0000")}, false},
		{"read-only", fields{modifierStateProgressOnly, AllClientSupportedActionMasks}, args{""}, nil, true},
		{"not-negotiated", fields{modifierStateReadWrite, 0}, args{""}, nil, true},
		{"fixup-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<>"}, &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("<>\u0000")}, false},
		{"fixup-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<\u0000>"}, &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("< >\u0000")}, false},
		{"fixup-3", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"<\r\n>"}, &wire.Message{Code: wire.Code(wire.ActDelRcpt), Data: []byte("< >\u0000")}, false},
		{"err-too-long", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{strings.Repeat(".", int(DataSize64K))}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.DeleteRecipient(tt.args.r); (err != nil) != tt.wantErr {
				t.Errorf("DeleteRecipient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DeleteRecipient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_InsertHeader(t *testing.T) {
	type fields struct {
		state   modifierState
		actions OptAction
	}
	type args struct {
		index int
		name  string
		value string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", "Test"}, &wire.Message{Code: wire.Code(wire.ActInsertHeader), Data: []byte("\u0000\u0000\u0000\u0001Subject\000Test\000")}, false},
		{"read-only", fields{modifierStateProgressOnly, AllClientSupportedActionMasks}, args{1, "Subject", "Test"}, nil, true},
		{"not-negotiated", fields{modifierStateReadWrite, 0}, args{1, "Subject", "Test"}, nil, true},
		{"invalid-index-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{-1, "Subject:", "Test"}, nil, true},
		{"invalid-index-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{math.MaxUint32 + 1, "Subject:", "Test"}, nil, true},
		{"invalid-name-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject:", "Test"}, nil, true},
		{"invalid-name-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, " Subject", "Test"}, nil, true},
		{"invalid-name-3", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subj\u0000ect", "Test"}, nil, true},
		{"invalid-name-4", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject💣", "Test"}, nil, true},
		{"invalid-name-5", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "", "Test"}, nil, true},
		{"fixup-value-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", "Test\r\n Line2"}, &wire.Message{Code: wire.Code(wire.ActInsertHeader), Data: []byte("\u0000\u0000\u0000\u0001Subject\000Test\n Line2\000")}, false},
		{"fixup-value-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", "Test\u0000ing"}, &wire.Message{Code: wire.Code(wire.ActInsertHeader), Data: []byte("\u0000\u0000\u0000\u0001Subject\000Test ing\000")}, false},
		{"err-too-long", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{1, "Subject", strings.Repeat(".", int(DataSize64K))}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.InsertHeader(tt.args.index, tt.args.name, tt.args.value); (err != nil) != tt.wantErr {
				t.Errorf("InsertHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("InsertHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_Progress(t *testing.T) {
	type fields struct {
		version uint32
		state   modifierState
		actions OptAction
	}
	tests := []struct {
		name    string
		fields  fields
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{6, modifierStateReadWrite, AllClientSupportedActionMasks}, &wire.Message{Code: wire.Code(wire.ActProgress)}, false},
		{"read-only", fields{6, modifierStateProgressOnly, AllClientSupportedActionMasks}, &wire.Message{Code: wire.Code(wire.ActProgress)}, false},
		{"wrong-version", fields{4, modifierStateReadWrite, 0}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				version:     tt.fields.version,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.Progress(); (err != nil) != tt.wantErr {
				t.Errorf("Progress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Progress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_Quarantine(t *testing.T) {
	type fields struct {
		state   modifierState
		actions OptAction
	}
	type args struct {
		reason string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"reason"}, &wire.Message{Code: wire.Code(wire.ActQuarantine), Data: []byte("reason\u0000")}, false},
		{"read-only", fields{modifierStateProgressOnly, AllClientSupportedActionMasks}, args{""}, nil, true},
		{"not-negotiated", fields{modifierStateReadWrite, 0}, args{""}, nil, true},
		{"fixup-1", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"reason\u0000"}, &wire.Message{Code: wire.Code(wire.ActQuarantine), Data: []byte("reason \u0000")}, false},
		{"fixup-2", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{"reason\r\nline2"}, &wire.Message{Code: wire.Code(wire.ActQuarantine), Data: []byte("reason line2\u0000")}, false},
		{"err-too-long", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{strings.Repeat(".", int(DataSize64K))}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.Quarantine(tt.args.reason); (err != nil) != tt.wantErr {
				t.Errorf("Quarantine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Quarantine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_ReplaceBodyRawChunk(t *testing.T) {
	type fields struct {
		state   modifierState
		actions OptAction
	}
	type args struct {
		chunk []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wire.Message
		wantErr bool
	}{
		{"allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{[]byte("body")}, &wire.Message{Code: wire.Code(wire.ActReplBody), Data: []byte("body")}, false},
		{"read-only", fields{modifierStateProgressOnly, AllClientSupportedActionMasks}, args{[]byte("body")}, nil, true},
		{"not-negotiated", fields{modifierStateReadWrite, 0}, args{[]byte("body")}, nil, true},
		{"nul-allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{[]byte("body\u0000with-nul")}, &wire.Message{Code: wire.Code(wire.ActReplBody), Data: []byte("body\u0000with-nul")}, false},
		{"too-big", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{bytes.Repeat([]byte("0123456789ABCDEF"), 4480)}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *wire.Message
			writePacket := func(msg *wire.Message) error {
				got = msg
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			if err := m.ReplaceBodyRawChunk(tt.args.chunk); (err != nil) != tt.wantErr {
				t.Errorf("ReplaceBodyRawChunk() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReplaceBodyRawChunk() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_ReplaceBody(t *testing.T) {
	bigBody := bytes.Repeat([]byte("0123456789ABCDEF"), 4480) // 16 * 4480 = 71680 = 70 KiB
	bigBodyPkt1 := bigBody[0:DataSize64K]
	bigBodyPkt2 := bigBody[DataSize64K:]
	type fields struct {
		state   modifierState
		actions OptAction
	}
	type args struct {
		writes [][]byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*wire.Message
		wantErr bool
	}{
		{"allowed", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{[][]byte{[]byte(("body"))}}, []*wire.Message{{Code: wire.Code(wire.ActReplBody), Data: []byte("body")}}, false},
		{"read-only", fields{modifierStateProgressOnly, AllClientSupportedActionMasks}, args{[][]byte{[]byte(("body"))}}, nil, true},
		{"not-negotiated", fields{modifierStateReadWrite, 0}, args{[][]byte{[]byte(("body"))}}, nil, true},
		{"least-packets", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{[][]byte{[]byte("body"), []byte(("body"))}}, []*wire.Message{{Code: wire.Code(wire.ActReplBody), Data: []byte("bodybody")}}, false},
		{"spill-over", fields{modifierStateReadWrite, AllClientSupportedActionMasks}, args{[][]byte{bigBody}}, []*wire.Message{{Code: wire.Code(wire.ActReplBody), Data: bigBodyPkt1}, {Code: wire.Code(wire.ActReplBody), Data: bigBodyPkt2}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []*wire.Message
			writePacket := func(msg *wire.Message) error {
				cpy := &wire.Message{Code: msg.Code, Data: make([]byte, len(msg.Data))}
				copy(cpy.Data, msg.Data)
				got = append(got, cpy)
				return nil
			}
			m := &modifier{
				macros:      NewMacroBag(),
				writePacket: writePacket,
				state:       tt.fields.state,
				actions:     tt.fields.actions,
				maxDataSize: DataSize64K,
			}
			r, w := io.Pipe()
			go func() {
				var err error
				for _, r := range tt.args.writes {
					_, err = w.Write(r)
					if err != nil {
						break
					}
				}
				_ = w.CloseWithError(err)
			}()
			if err := m.ReplaceBody(r); (err != nil) != tt.wantErr {
				t.Errorf("ReplaceBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Got:")
				for i, m := range got {
					t.Error(fmt.Sprintf("%d - len %d\n", i, len(m.Data)) + hex.Dump(m.Data))
				}
				t.Errorf("Want:")
				for i, m := range tt.want {
					t.Error(fmt.Sprintf("%d - len %d\n", i, len(m.Data)) + hex.Dump(m.Data))
				}
			}
		})
	}
}

func Test_modifier_Version(t *testing.T) {
	type fields struct {
		version uint32
	}
	tests := []struct {
		name   string
		fields fields
		want   uint32
	}{
		{"works", fields{1234567890}, 1234567890},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &modifier{
				version: tt.fields.version,
			}
			if got := m.Version(); got != tt.want {
				t.Errorf("Version() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_Protocol(t *testing.T) {
	type fields struct {
		protocol OptProtocol
	}
	tests := []struct {
		name   string
		fields fields
		want   OptProtocol
	}{
		{"works", fields{1234567890}, 1234567890},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &modifier{
				protocol: tt.fields.protocol,
			}
			if got := m.Protocol(); got != tt.want {
				t.Errorf("Protocol() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_Actions(t *testing.T) {
	type fields struct {
		actions OptAction
	}
	tests := []struct {
		name   string
		fields fields
		want   OptAction
	}{
		{"works", fields{1234567890}, 1234567890},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &modifier{
				actions: tt.fields.actions,
			}
			if got := m.Actions(); got != tt.want {
				t.Errorf("Actions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_MaxDataSize(t *testing.T) {
	type fields struct {
		maxDataSize DataSize
	}
	tests := []struct {
		name   string
		fields fields
		want   DataSize
	}{
		{"works", fields{1234567890}, 1234567890},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &modifier{
				maxDataSize: tt.fields.maxDataSize,
			}
			if got := m.MaxDataSize(); got != tt.want {
				t.Errorf("MaxDataSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_MilterId(t *testing.T) {
	type fields struct {
		milterId uint64
	}
	tests := []struct {
		name   string
		fields fields
		want   uint64
	}{
		{"works", fields{1234567890}, 1234567890},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &modifier{
				milterId: tt.fields.milterId,
			}
			if got := m.MilterId(); got != tt.want {
				t.Errorf("MilterId() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_modifier_GetEx(t *testing.T) {
	macroBag := NewMacroBag()
	macroBag.Set(MacroAuthAuthen, "value")
	type fields struct {
		macros Macros
	}
	type args struct {
		name MacroName
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
		wantOk bool
	}{
		{"works", fields{macroBag}, args{MacroAuthAuthen}, "value", true},
		{"not-found", fields{macroBag}, args{MacroAuthType}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &modifier{
				macros: tt.fields.macros,
			}
			got, ok := m.GetEx(tt.args.name)
			if got != tt.want {
				t.Errorf("GetEx() got = %v, want %v", got, tt.want)
			}
			if ok != tt.wantOk {
				t.Errorf("GetEx() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func Test_modifier_Get(t *testing.T) {
	macroBag := NewMacroBag()
	macroBag.Set(MacroAuthAuthen, "value")
	type fields struct {
		macros Macros
	}
	type args struct {
		name MacroName
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{"works", fields{macroBag}, args{MacroAuthAuthen}, "value"},
		{"not-found", fields{macroBag}, args{MacroAuthType}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &modifier{
				macros: tt.fields.macros,
			}
			if got := m.Get(tt.args.name); got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}
