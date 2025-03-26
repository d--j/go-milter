package mailfilter

import (
	"reflect"
	"testing"
)

func Test_decision_Equal(t *testing.T) {
	type args struct {
		d2 Decision
	}
	tests := []struct {
		name string
		d    decision
		args args
		want bool
	}{
		{"works1", Accept, args{Accept}, true},
		{"works2", Accept, args{Reject}, false},
		{"acceptNil", Accept, args{nil}, false},
		{"equalsCustom", Reject, args{CustomErrorResponse(550, "5.7.1 Command rejected")}, true},
		{"equalsCustom1", TempFail, args{CustomErrorResponse(550, "5.7.1 Command rejected")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Equal(tt.args.d2); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_customResponse_String(t *testing.T) {
	type fields struct {
		code   uint16
		reason string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"Empty", fields{400, ""}, "400 "},
		{"Simple", fields{400, "Line 1"}, "400 Line 1"},
		{"Multi", fields{400, "4.0.0 Line 1\nLine 2"}, "400-4.0.0 Line 1\r\n400 4.0.0 Line 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := customResponse{
				code:   tt.fields.code,
				reason: tt.fields.reason,
			}
			if got := c.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("PanicOnWrongCode", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("String() did not panic")
			}
		}()
		c := customResponse{99, ""}
		if got := c.String(); got != "" {
			t.Errorf("String() = %v, want %v", got, "")
		}
	})

}

func Test_customResponse_Equal(t *testing.T) {
	type fields struct {
		code   uint16
		reason string
	}
	type args struct {
		d Decision
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{"works1", fields{400, "test"}, args{&customResponse{400, "test"}}, true},
		{"works2", fields{400, "test"}, args{&customResponse{400, "test1"}}, false},
		{"works3", fields{400, "test"}, args{nil}, false},
		{"default reject", fields{550, "5.7.1 Command rejected"}, args{Reject}, true},
		{"default reject != temp fail", fields{550, "5.7.1 Command rejected"}, args{TempFail}, false},
		{"default temp fail", fields{451, "4.7.1 Service unavailable - try again later"}, args{TempFail}, true},
		{"default temp fail != reject", fields{451, "4.7.1 Service unavailable - try again later"}, args{Reject}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := customResponse{
				code:   tt.fields.code,
				reason: tt.fields.reason,
			}
			if got := c.Equal(tt.args.d); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
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
			if got.String() != Accept.String() {
				t.Errorf("QuarantineResponse().String() = %v, want %v", got.String(), Accept.String())
			}
			if !got.Equal(Accept) {
				t.Errorf("QuarantineResponse().Equal(Accept) = false, want true")
			}
		})
	}
}
