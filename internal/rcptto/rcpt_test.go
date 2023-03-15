package rcptto

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/d--j/go-milter/mailfilter/addr"
)

func Test_calculateRcptToDiff(t *testing.T) {
	t.Parallel()
	type args struct {
		orig    []*addr.RcptTo
		changed []*addr.RcptTo
	}
	tests := []struct {
		name          string
		args          args
		wantDeletions []*addr.RcptTo
		wantAdditions []*addr.RcptTo
	}{
		{"nil", args{nil, nil}, nil, nil},
		{"empty", args{[]*addr.RcptTo{}, []*addr.RcptTo{}}, nil, nil},
		{"remove", args{[]*addr.RcptTo{addr.NewRcptTo("one", "", "")}, []*addr.RcptTo{}}, []*addr.RcptTo{addr.NewRcptTo("one", "", "")}, nil},
		{"add", args{[]*addr.RcptTo{}, []*addr.RcptTo{addr.NewRcptTo("one", "", "")}}, nil, []*addr.RcptTo{addr.NewRcptTo("one", "", "")}},
		{"add double", args{[]*addr.RcptTo{}, []*addr.RcptTo{addr.NewRcptTo("one", "", ""), addr.NewRcptTo("one", "", "")}}, nil, []*addr.RcptTo{addr.NewRcptTo("one", "", "")}},
		{"change", args{[]*addr.RcptTo{addr.NewRcptTo("one", "", "")}, []*addr.RcptTo{addr.NewRcptTo("one", "A=B", "")}}, []*addr.RcptTo{addr.NewRcptTo("one", "", "")}, []*addr.RcptTo{addr.NewRcptTo("one", "A=B", "")}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotDeletions, gotAdditions := Diff(tt.args.orig, tt.args.changed)
			if !reflect.DeepEqual(gotDeletions, tt.wantDeletions) {
				t.Errorf("calculateRcptToDiff() gotDeletions = %v, want %v", gotDeletions, tt.wantDeletions)
			}
			if !reflect.DeepEqual(gotAdditions, tt.wantAdditions) {
				t.Errorf("calculateRcptToDiff() gotAdditions = %v, want %v", gotAdditions, tt.wantAdditions)
			}
		})
	}
}

func TestHas(t *testing.T) {
	t.Parallel()
	type args struct {
		rcptTos []*addr.RcptTo
		rcptTo  string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"has", args{[]*addr.RcptTo{addr.NewRcptTo("root", "", "")}, "root"}, true},
		{"has not", args{[]*addr.RcptTo{addr.NewRcptTo("root", "", "")}, "toor"}, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Has(tt.args.rcptTos, tt.args.rcptTo); got != tt.want {
				t.Errorf("Has() = %v, want %v", got, tt.want)
			}
		})
	}
}

func cmp(one, two []*addr.RcptTo) bool {
	if (one == nil) != (two == nil) || len(one) != len(two) {
		return false
	}
	for i, r := range one {
		if two[i].Addr != r.Addr || two[i].Args != r.Args || two[i].Transport() != r.Transport() {
			return false
		}
	}
	return true
}

func out(in []*addr.RcptTo) string {
	if in == nil {
		return "<nil>"
	}
	var s strings.Builder
	s.WriteString("[")
	for i, r := range in {
		if i > 0 {
			s.WriteString(",")
		}
		s.WriteString(fmt.Sprintf("{Addr: %q, Args: %q, transport: %q}", r.Addr, r.Args, r.Transport()))
	}
	s.WriteString("]")
	return s.String()
}

func TestAdd(t *testing.T) {
	t.Parallel()
	type args struct {
		rcptTos   []*addr.RcptTo
		rcptTo    string
		esmtpArgs string
	}
	tests := []struct {
		name    string
		args    args
		wantOut []*addr.RcptTo
	}{
		{"add1", args{nil, "root", "A=B"}, []*addr.RcptTo{addr.NewRcptTo("root", "A=B", "new")}},
		{"add2", args{[]*addr.RcptTo{addr.NewRcptTo("root", "", "smtp")}, "toor", "A=B"}, []*addr.RcptTo{addr.NewRcptTo("root", "", "smtp"), addr.NewRcptTo("toor", "A=B", "new")}},
		{"change", args{[]*addr.RcptTo{addr.NewRcptTo("root", "", "smtp")}, "root", "A=B"}, []*addr.RcptTo{addr.NewRcptTo("root", "A=B", "smtp")}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if gotOut := Add(tt.args.rcptTos, tt.args.rcptTo, tt.args.esmtpArgs); !cmp(gotOut, tt.wantOut) {
				t.Errorf("Add() = %v, want %v", out(gotOut), out(tt.wantOut))
			}
		})
	}
}

func TestDel(t *testing.T) {
	t.Parallel()
	type args struct {
		rcptTos []*addr.RcptTo
		rcptTo  string
	}
	tests := []struct {
		name    string
		args    args
		wantOut []*addr.RcptTo
	}{
		{"nil ok", args{nil, "root"}, nil},
		{"empty ok", args{[]*addr.RcptTo{}, "root"}, []*addr.RcptTo{}},
		{"not-found", args{[]*addr.RcptTo{addr.NewRcptTo("root", "", "smtp")}, "toor"}, []*addr.RcptTo{addr.NewRcptTo("root", "", "smtp")}},
		{"found", args{[]*addr.RcptTo{addr.NewRcptTo("root", "", "smtp")}, "root"}, []*addr.RcptTo{}},
		{"found2", args{[]*addr.RcptTo{addr.NewRcptTo("root", "", "smtp"), addr.NewRcptTo("toor", "", "smtp")}, "root"}, []*addr.RcptTo{addr.NewRcptTo("toor", "", "smtp")}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if gotOut := Del(tt.args.rcptTos, tt.args.rcptTo); !cmp(gotOut, tt.wantOut) {
				t.Errorf("Del() = %v, want %v", out(gotOut), out(tt.wantOut))
			}
		})
	}
}

func TestCopy(t *testing.T) {
	t.Parallel()
	if got := Copy(nil); !reflect.DeepEqual(got, []*addr.RcptTo{}) {
		t.Errorf("Copy(nil) = %v, want %v", got, []*addr.RcptTo{})
	}
	r1 := addr.NewRcptTo("root", "", "")
	got := Copy([]*addr.RcptTo{r1})
	if got[0] == r1 {
		t.Errorf("Copy() did not create an independent copy")
	}
}
