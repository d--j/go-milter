package mailfilter

import (
	"reflect"
	"testing"
)

func TestCustomErrorResponse(t *testing.T) {
	type args struct {
		code   uint16
		reason string
	}
	tests := []struct {
		name string
		args args
		want Decision
	}{
		{"works", args{400, "test"}, &customResponse{400, "test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CustomErrorResponse(tt.args.code, tt.args.reason); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CustomErrorResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_customResponse_getCode(t *testing.T) {
	type fields struct {
		code   uint16
		reason string
	}
	tests := []struct {
		name   string
		fields fields
		want   uint16
	}{
		{"works", fields{400, "test"}, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := customResponse{
				code:   tt.fields.code,
				reason: tt.fields.reason,
			}
			if got := c.getCode(); got != tt.want {
				t.Errorf("getCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_customResponse_getReason(t *testing.T) {
	type fields struct {
		code   uint16
		reason string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"works", fields{400, "test"}, "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := customResponse{
				code:   tt.fields.code,
				reason: tt.fields.reason,
			}
			if got := c.getReason(); got != tt.want {
				t.Errorf("getReason() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_decision_getCode(t *testing.T) {
	tests := []struct {
		name string
		d    decision
		want uint16
	}{
		{"works", Accept, 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.getCode(); got != tt.want {
				t.Errorf("getCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_decision_getReason(t *testing.T) {
	tests := []struct {
		name string
		d    decision
		want string
	}{
		{"works", Accept, "accept"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.getReason(); got != tt.want {
				t.Errorf("getReason() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuarantineResponse(t *testing.T) {
	type args struct {
		reason string
	}
	tests := []struct {
		name string
		args args
		want Decision
	}{
		{"works", args{"reason"}, &quarantineResponse{reason: "reason"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuarantineResponse(tt.args.reason)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("QuarantineResponse() = %v, want %v", got, tt.want)
			}
			if got.getCode() != Accept.getCode() {
				t.Errorf("QuarantineResponse().getCode() = %v, want %v", got.getCode(), Accept.getCode())
			}
			expectedReason := "accept (quarantined: \"reason\")"
			if got.getReason() != expectedReason {
				t.Errorf("QuarantineResponse().getReason() = %v, want %v", got.getReason(), expectedReason)
			}
		})
	}
}

func Test_quarantineResponse_getReason(t *testing.T) {
	type fields struct {
		reason string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"empty", fields{""}, "accept (quarantined)"},
		{"simple", fields{"reason"}, "accept (quarantined)"},
		{"EEC", fields{"2.6.0 I think this is spam"}, "accept (quarantined)"},
		{"EEC err", fields{"4.6.0 I think this is spam"}, "accept (quarantined)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := quarantineResponse{
				reason: tt.fields.reason,
			}
			if got := c.getReason(); got != tt.want {
				t.Errorf("getReason() = %v, want %v", got, tt.want)
			}
		})
	}
}
