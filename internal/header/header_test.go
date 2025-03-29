package header

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/emersion/go-message/mail"
)

func testHeader() *Header {
	return &Header{fields: []*Field{
		{
			Index:        0,
			CanonicalKey: "From",
			Raw:          []byte("From: <root@localhost>"),
		},
		{
			Index:        1,
			CanonicalKey: "To",
			Raw:          []byte("To:  <root@localhost>, <nobody@localhost>"),
		},
		{
			Index:        2,
			CanonicalKey: "Subject",
			Raw:          []byte("subject: =?UTF-8?Q?=F0=9F=9F=A2?="), // 🟢
		},
		{
			Index:        3,
			CanonicalKey: "Date",
			Raw:          []byte("DATE:\tWed, 01 Mar 2023 15:47:33 +0100"),
		},
	}}
}

func Test_unfold(t *testing.T) {
	tests := []struct {
		lines string
		want  string
	}{
		{"one", "one"},
		{"one\ntwo", "one two"},
		{"one\n two", "one two"},
		{"one\n\ttwo", "one two"},
		{"one\r\ntwo", "one two"},
		{"one\r\n\ttwo", "one two"},
		{"one\r\n two", "one two"},
		{"one\r\n  two", "one two"},
		{"one\r\n \t two", "one two"},
	}
	for _, tt := range tests {
		t.Run(tt.lines, func(t *testing.T) {
			if got := unfold(tt.lines); got != tt.want {
				t.Errorf("unfold() = %v, want %v", got, tt.want)
			}
		})
	}
}

var root, nobody = mail.Address{
	Name:    "",
	Address: "root@localhost",
}, mail.Address{
	Name:    "",
	Address: "nobody@localhost",
}

func TestHeaderFields_CanonicalKey(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"From", fields{0, testHeader()}, "From"},
		{"To", fields{1, testHeader()}, "To"},
		{"Subject", fields{2, testHeader()}, "Subject"},
		{"Date", fields{3, testHeader()}, "Date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.CanonicalKey(); got != tt.want {
				t.Errorf("CanonicalKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_Del(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   *Field
	}{
		{"First", fields{0, testHeader()}, &Field{0, "From", []byte("From:"), true}},
		{"Third", fields{2, testHeader()}, &Field{2, "Subject", []byte("subject:"), true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper()}
			f.Del()
			got := f.h.fields[f.index()]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Del() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_Value(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"From", fields{0, testHeader()}, " <root@localhost>"},
		{"To", fields{1, testHeader()}, "  <root@localhost>, <nobody@localhost>"},
		{"Subject", fields{2, testHeader()}, " =?UTF-8?Q?=F0=9F=9F=A2?="},
		{"Date", fields{3, testHeader()}, "\tWed, 01 Mar 2023 15:47:33 +0100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.Value(); got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_UnfoldedValue(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"From", fields{0, testHeader()}, " <root@localhost>"},
		{"To", fields{1, testHeader()}, "  <root@localhost>, <nobody@localhost>"},
		{"Subject", fields{2, testHeader()}, " =?UTF-8?Q?=F0=9F=9F=A2?="},
		{"Date", fields{3, testHeader()}, "\tWed, 01 Mar 2023 15:47:33 +0100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.UnfoldedValue(); got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_GetAddressList(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name    string
		fields  fields
		want    []*mail.Address
		wantErr bool
	}{
		{"From", fields{0, testHeader()}, []*mail.Address{&root}, false},
		{"To", fields{1, testHeader()}, []*mail.Address{&root, &nobody}, false},
		{"Subject", fields{2, testHeader()}, nil, true},
		{"Date", fields{3, testHeader()}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			got, err := f.AddressList()
			if (err != nil) != tt.wantErr {
				t.Errorf("AddressList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddressList() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_GetText(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		{"From", fields{0, testHeader()}, " <root@localhost>", false},
		{"To", fields{1, testHeader()}, "  <root@localhost>, <nobody@localhost>", false},
		{"Subject", fields{2, testHeader()}, " 🟢", false},
		{"Date", fields{3, testHeader()}, "\tWed, 01 Mar 2023 15:47:33 +0100", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			got, err := f.Text()
			if (err != nil) != tt.wantErr {
				t.Errorf("Text() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Text() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func outputFields(fields []*Field) string {
	h := Header{fields: fields}
	b, _ := io.ReadAll(h.Reader())
	return string(b)
}

func TestHeaderFields_InsertAfter(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	type args struct {
		key   string
		value string
	}
	addOne := []args{{"Test", "one"}}
	expectOne := []*Field{{-1, "Test", []byte("Test: one"), false}}
	addTwo := []args{{"Test", "one"}, {"Test", "two"}}
	expectTwo := []*Field{{-1, "Test", []byte("Test: one"), false}, {-1, "Test", []byte("Test: two"), false}}
	tests := []struct {
		name     string
		fields   fields
		args     []args
		want     []*Field
		wantSkip int
		wantNext bool
	}{
		{"From", fields{0, testHeader()}, addOne, append(testHeader().fields[:1], append(expectOne, testHeader().fields[1:]...)...), 1, true},
		{"To", fields{1, testHeader()}, addOne, append(testHeader().fields[:2], append(expectOne, testHeader().fields[2:]...)...), 1, true},
		{"Subject", fields{2, testHeader()}, addOne, append(testHeader().fields[:3], append(expectOne, testHeader().fields[3:]...)...), 1, true},
		{"Date", fields{3, testHeader()}, addOne, append(testHeader().fields[:4], append(expectOne, testHeader().fields[4:]...)...), 1, false},
		{"From2", fields{0, testHeader()}, addTwo, append(testHeader().fields[:1], append(expectTwo, testHeader().fields[1:]...)...), 2, true},
		{"Date2", fields{3, testHeader()}, addTwo, append(testHeader().fields[:4], append(expectTwo, testHeader().fields[4:]...)...), 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			for _, arg := range tt.args {
				f.InsertAfter(arg.key, arg.value)
			}
			got := tt.fields.h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("InsertAfter() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
			gotSkip := f.skip
			if gotSkip != tt.wantSkip {
				t.Errorf("InsertAfter() = %v, wantSkip %v", gotSkip, tt.wantSkip)
			}
			gotNext := f.Next()
			if gotNext != tt.wantNext {
				t.Errorf("InsertAfter() = %v, want %v", gotNext, tt.wantNext)
			}
		})
	}
}

func TestHeaderFields_InsertBefore(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	type args struct {
		key   string
		value string
	}
	addOne := []args{{"Test", "one"}}
	expectOne := []*Field{{-1, "Test", []byte("Test: one"), false}}
	addTwo := []args{{"Test", "one"}, {"Test", "two"}}
	expectTwo := []*Field{{-1, "Test", []byte("Test: one"), false}, {-1, "Test", []byte("Test: two"), false}}
	tests := []struct {
		name     string
		fields   fields
		args     []args
		want     []*Field
		wantSkip int
		wantNext bool
	}{
		{"From", fields{0, testHeader()}, addOne, append(testHeader().fields[:0], append(expectOne, testHeader().fields[0:]...)...), 0, true},
		{"To", fields{1, testHeader()}, addOne, append(testHeader().fields[:1], append(expectOne, testHeader().fields[1:]...)...), 0, true},
		{"Subject", fields{2, testHeader()}, addOne, append(testHeader().fields[:2], append(expectOne, testHeader().fields[2:]...)...), 0, true},
		{"Date", fields{3, testHeader()}, addOne, append(testHeader().fields[:3], append(expectOne, testHeader().fields[3:]...)...), 0, false},
		{"From2", fields{0, testHeader()}, addTwo, append(testHeader().fields[:0], append(expectTwo, testHeader().fields[0:]...)...), 0, true},
		{"Date2", fields{3, testHeader()}, addTwo, append(testHeader().fields[:3], append(expectTwo, testHeader().fields[3:]...)...), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			for _, arg := range tt.args {
				f.InsertBefore(arg.key, arg.value)
			}
			got := tt.fields.h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("InsertBefore() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
			gotSkip := f.skip
			if gotSkip != tt.wantSkip {
				t.Errorf("InsertAfter() = %v, wantSkip %v", gotSkip, tt.wantSkip)
			}
			gotNext := f.Next()
			if gotNext != tt.wantNext {
				t.Errorf("InsertAfter() = %v, want %v", gotNext, tt.wantNext)
			}
		})
	}
}

func TestHeaderFields_Key(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"From", fields{0, testHeader()}, "From"},
		{"To", fields{1, testHeader()}, "To"},
		{"Subject", fields{2, testHeader()}, "subject"},
		{"Date", fields{3, testHeader()}, "DATE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.Key(); got != tt.want {
				t.Errorf("Key() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_Len(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   int
	}{
		{"works", fields{0, testHeader()}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.Len(); got != tt.want {
				t.Errorf("Len() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_Next(t *testing.T) {
	type fields struct {
		cursor int
		skip   int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{"From", fields{0, 0, testHeader()}, true},
		{"To", fields{1, 0, testHeader()}, true},
		{"Subject", fields{2, 0, testHeader()}, true},
		{"Date", fields{3, 0, testHeader()}, false},
		{"From1", fields{0, 1, testHeader()}, true},
		{"To1", fields{1, 1, testHeader()}, true},
		{"Subject1", fields{2, 1, testHeader()}, false},
		{"Date1", fields{3, 1, testHeader()}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				skip:   tt.fields.skip,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.Next(); got != tt.want {
				t.Errorf("Next() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_Raw(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	tests := []struct {
		name   string
		fields fields
		want   []byte
	}{
		{"From", fields{0, testHeader()}, []byte("From: <root@localhost>")},
		{"To", fields{1, testHeader()}, []byte("To:  <root@localhost>, <nobody@localhost>")},
		{"Subject", fields{2, testHeader()}, []byte("subject: =?UTF-8?Q?=F0=9F=9F=A2?=")},
		{"Date", fields{3, testHeader()}, []byte("DATE:\tWed, 01 Mar 2023 15:47:33 +0100")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.Raw(); !bytes.Equal(got, tt.want) {
				t.Errorf("Key() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_Replace(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	type args struct {
		key   string
		value string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []*Field
	}{
		{"works", fields{0, testHeader()}, args{"new", "header"}, append([]*Field{{0, "New", []byte("new: header"), false}}, testHeader().fields[1:]...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			f.Replace(tt.args.key, tt.args.value)
			got := tt.fields.h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Replace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_Set(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	type args struct {
		value string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *Field
	}{
		{"First", fields{0, testHeader()}, args{"set"}, &Field{0, "From", []byte("From: set"), false}},
		{"Third", fields{2, testHeader()}, args{"\tset"}, &Field{2, "Subject", []byte("subject:\tset"), false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			f.Set(tt.args.value)
			got := f.h.fields[f.index()]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Set() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_SetAddressList(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	type args struct {
		value []*mail.Address
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *Field
	}{
		{"One", fields{0, testHeader()}, args{[]*mail.Address{&nobody}}, &Field{0, "From", []byte("From: <nobody@localhost>"), false}},
		{"Two", fields{1, testHeader()}, args{[]*mail.Address{&nobody, &root}}, &Field{1, "To", []byte("To: <nobody@localhost>,\r\n <root@localhost>"), false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			f.SetAddressList(tt.args.value)
			got := f.h.fields[f.index()]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetAddressList() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHeaderFields_SetText(t *testing.T) {
	type fields struct {
		cursor int
		h      *Header
	}
	type args struct {
		value string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *Field
	}{
		{"Set", fields{0, testHeader()}, args{"set"}, &Field{0, "From", []byte("From: set"), false}},
		{"UTF-8", fields{2, testHeader()}, args{"🔴"}, &Field{2, "Subject", []byte("subject: =?utf-8?q?=F0=9F=94=B4?="), false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			f.SetText(tt.args.value)
			got := f.h.fields[f.index()]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetText() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_Add(t *testing.T) {
	type args struct {
		key   string
		value string
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   []*Field
	}{
		{"works", testHeader().fields, args{"key", "value"}, append(testHeader().fields, &Field{-1, "Key", []byte("key: value"), false})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.Add(tt.args.key, tt.args.value)
			got := h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Add() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
		})
	}
}

func TestHeader_Date(t *testing.T) {
	brokenDate := testHeader()
	brokenDate.fields[3].Raw = []byte("Date: broken")
	tests := []struct {
		name    string
		fields  []*Field
		want    time.Time
		wantErr bool
	}{
		{"Date", testHeader().fields, time.Date(2023, time.March, 1, 15, 47, 33, 0, time.FixedZone("CET", 60*60)), false},
		{"Broken", brokenDate.fields, time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			got, err := h.Date()
			if (err != nil) != tt.wantErr {
				t.Errorf("Date() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("Date() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_Fields(t *testing.T) {
	h := testHeader()
	fields := h.Fields()
	for fields.Next() {
		if fields.CanonicalKey() == "From" {
			fields.InsertBefore("X-From1", fields.Value())
			fields.InsertBefore("X-From2", fields.Value())
			if fields.CanonicalKey() != "From" {
				t.Error("InsertBefore changed cursor")
			}
		}

		if fields.CanonicalKey() == "To" {
			fields.InsertAfter("X-After1", "value1")
			fields.InsertAfter("X-After2", "value2")
			if fields.CanonicalKey() != "To" {
				t.Error("InsertAfter changed cursor")
			}
		}

		if fields.CanonicalKey() == "Subject" {
			fields.Del()
		}

		switch fields.CanonicalKey() {
		case "X-From1", "X-From2", "X-After1", "X-After2":
			t.Error("iterated over inserted key", fields.CanonicalKey())
		}
	}

	if fields.Next() {
		t.Error("Next() call should return false")
	}

	b, _ := io.ReadAll(h.Reader())
	got := string(b)
	expect := "X-From1: <root@localhost>\r\nX-From2: <root@localhost>\r\nFrom: <root@localhost>\r\nTo:  <root@localhost>, <nobody@localhost>\r\nX-After1: value1\r\nX-After2: value2\r\nDATE:\tWed, 01 Mar 2023 15:47:33 +0100\r\n\r\n"
	if got != expect {
		t.Errorf("got %q, expect %q", got, expect)
	}

	fields2 := h.Fields()
	for fields2.Next() {
		if fields2.CanonicalKey() == "X-From1" {
			fields2.SetAddressList([]*mail.Address{&nobody})
			fields2.ReplaceAddressList("X-To", []*mail.Address{&nobody, &root})
			fields2.ReplaceText("X-Text", "🟡")
		}
		if fields2.CanonicalKey() == "Subject" {
			if !fields2.IsDeleted() {
				t.Error("Subject should be deleted")
			}
			// before/after order scrambled on purpose
			fields2.InsertTextBefore("X-Before1", "🟡")
			fields2.InsertTextAfter("X-After1", "🟡")
			fields2.InsertAddressListAfter("X-After2", []*mail.Address{&nobody})
			fields2.InsertAddressListBefore("X-Before2", []*mail.Address{&nobody})
		}
		switch fields2.CanonicalKey() {
		case "X-From2", "X-After1", "X-After2":
			fields2.Del()
		}
	}

	b, _ = io.ReadAll(h.Reader())
	got = string(b)
	expect = "X-Text: =?utf-8?q?=F0=9F=9F=A1?=\r\nFrom: <root@localhost>\r\nTo:  <root@localhost>, <nobody@localhost>\r\nX-Before1: =?utf-8?q?=F0=9F=9F=A1?=\r\nX-Before2: <nobody@localhost>\r\nX-After1: =?utf-8?q?=F0=9F=9F=A1?=\r\nX-After2: <nobody@localhost>\r\nDATE:\tWed, 01 Mar 2023 15:47:33 +0100\r\n\r\n"
	if got != expect {
		t.Errorf("got %q, expect %q", got, expect)
	}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic")
		}
	}()
	fields2.Value()
}

func TestHeader_Value(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   string
	}{
		{"works", testHeader().fields, args{"fRoM"}, " <root@localhost>"},
		{"not found", testHeader().fields, args{"not-there"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			if got := h.Value(tt.args.key); got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_UnfoldedValue(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   string
	}{
		{"works", testHeader().fields, args{"fRoM"}, " <root@localhost>"},
		{"not found", testHeader().fields, args{"not-there"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			if got := h.UnfoldedValue(tt.args.key); got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_GetAddressList(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  []*Field
		args    args
		want    []*mail.Address
		wantErr bool
	}{
		{"From", testHeader().fields, args{"fRoM"}, []*mail.Address{&root}, false},
		{"To", testHeader().fields, args{"tO"}, []*mail.Address{&root, &nobody}, false},
		{"Subject", testHeader().fields, args{"SUBJECT"}, nil, true},
		{"Date", testHeader().fields, args{"Date"}, nil, true},
		{"Unknown", testHeader().fields, args{"Unknown"}, []*mail.Address{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			got, err := h.AddressList(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddressList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddressList() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_GetText(t *testing.T) {
	brokenSubject := testHeader()
	brokenSubject.fields[2].Raw = []byte("Subject: =?e-404?Q?=F0=9F=9F=A2?=")
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  []*Field
		args    args
		want    string
		wantErr bool
	}{
		{"works", testHeader().fields, args{"subJeCt"}, " 🟢", false},
		{"broken", brokenSubject.fields, args{"subJeCt"}, " =?e-404?Q?=F0=9F=9F=A2?=", true},
		{"Unknown", testHeader().fields, args{"Unknown"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			got, err := h.Text(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Text() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Text() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_Reader(t *testing.T) {
	tests := []struct {
		name   string
		fields []*Field
		want   string
	}{
		{"works", testHeader().fields, "From: <root@localhost>\r\nTo:  <root@localhost>, <nobody@localhost>\r\nsubject: =?UTF-8?Q?=F0=9F=9F=A2?=\r\nDATE:\tWed, 01 Mar 2023 15:47:33 +0100\r\n\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			b, err := io.ReadAll(h.Reader())
			if err != nil {
				t.Fatal(err)
			}
			got := string(b)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reader() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHeader_Set(t *testing.T) {
	type args struct {
		key   string
		value string
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   []*Field
	}{
		{"found", testHeader().fields, args{"suBJect", "value"}, append(testHeader().fields[:2], append([]*Field{{2, "Subject", []byte("subject: value"), false}}, testHeader().fields[3:]...)...)},
		{"not-found", testHeader().fields, args{"x-spam", "value"}, append(testHeader().fields, &Field{-1, "X-Spam", []byte("x-spam: value"), false})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.Set(tt.args.key, tt.args.value)
			got := h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Set() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
		})
	}
}

func TestHeader_SetAddressList(t *testing.T) {
	type args struct {
		key       string
		addresses []*mail.Address
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   []*Field
	}{
		{"works", testHeader().fields, args{"x-to", []*mail.Address{&root}}, append(testHeader().fields, &Field{-1, "X-To", []byte("x-to: <root@localhost>"), false})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.SetAddressList(tt.args.key, tt.args.addresses)
			got := h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetAddressList() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
		})
	}
}

func TestHeader_SetDate(t *testing.T) {
	type args struct {
		value time.Time
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   []*Field
	}{
		{"works", testHeader().fields, args{time.Date(1980, time.January, 1, 12, 0, 0, 0, time.UTC)}, append(testHeader().fields[:3], &Field{3, "Date", []byte("DATE: Tue, 01 Jan 1980 12:00:00 +0000"), false})},
		{"zero-ok", testHeader().fields, args{time.Time{}}, append(testHeader().fields[:3], &Field{3, "Date", []byte("DATE:"), true})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.SetDate(tt.args.value)
			got := h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetDate() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
		})
	}
}

func TestHeader_SetSubject(t *testing.T) {
	type args struct {
		value string
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   []*Field
	}{
		{"works", testHeader().fields, args{"set"}, append(testHeader().fields[:2], &Field{2, "Subject", []byte("subject: set"), false}, testHeader().fields[3])},
		{"zero-ok", testHeader().fields, args{""}, append(testHeader().fields[:2], &Field{2, "Subject", []byte("subject:"), true}, testHeader().fields[3])},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.SetSubject(tt.args.value)
			got := h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetSubject() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
		})
	}
}

func TestHeader_SetText(t *testing.T) {
	type args struct {
		key   string
		value string
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   []*Field
	}{
		{"works", testHeader().fields, args{"SubJect", "set"}, append(testHeader().fields[:2], &Field{2, "Subject", []byte("subject: set"), false}, testHeader().fields[3])},
		{"zero-ok", testHeader().fields, args{"Subject", ""}, append(testHeader().fields[:2], &Field{2, "Subject", []byte("subject:"), true}, testHeader().fields[3])},
		{"add", testHeader().fields, args{"x-red", "🔴"}, append(testHeader().fields, &Field{-1, "X-Red", []byte("x-red: =?utf-8?q?=F0=9F=94=B4?="), false})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.SetText(tt.args.key, tt.args.value)
			got := h.fields
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetText() = %q, want %q", outputFields(got), outputFields(tt.want))
			}
		})
	}
}

func TestHeader_Subject(t *testing.T) {
	brokenSubject := testHeader()
	brokenSubject.fields[2].Raw = []byte("Subject: =?e-404?Q?=F0=9F=9F=A2?=")
	tests := []struct {
		name    string
		fields  []*Field
		want    string
		wantErr bool
	}{
		{"works", testHeader().fields, " 🟢", false},
		{"broken", brokenSubject.fields, " =?e-404?Q?=F0=9F=9F=A2?=", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			got, err := h.Subject()
			if (err != nil) != tt.wantErr {
				t.Errorf("Subject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Subject() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_addRaw(t *testing.T) {
	type args struct {
		key string
		raw string
	}
	tests := []struct {
		name   string
		fields []*Field
		args   args
		want   []*Field
	}{
		{"works", nil, args{key: "TEST", raw: "TEST: value"}, []*Field{{CanonicalKey: "Test", Raw: []byte("TEST: value")}}},
		{"empty-is-ok", nil, args{key: "TEST", raw: "TEST:"}, []*Field{{CanonicalKey: "Test", Raw: []byte("TEST:")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.AddRaw(tt.args.key, []byte(tt.args.raw))
		})
	}
}

func TestHeader_copy(t *testing.T) {
	h := Header{fields: []*Field{{0, "Test", []byte("Test:"), false}}}
	h2 := h.Copy()
	h.fields[0].CanonicalKey = "Changed"
	if len(h2.fields) != len(h.fields) {
		t.Fatal("did not copy fields")
	}
	if h.fields[0].CanonicalKey == h2.fields[0].CanonicalKey {
		t.Fatal("did not copy deep copy fields")
	}
}

func Test_getRaw(t *testing.T) {
	type args struct {
		key   string
		value string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"empty", args{"TO", ""}, "TO:"},
		{"no space", args{"TO", "<root@localhost>"}, "TO: <root@localhost>"},
		{"space", args{"TO", " <root@localhost>"}, "TO: <root@localhost>"},
		{"tab", args{"TO", "\t<root@localhost>"}, "TO:\t<root@localhost>"},
		{"two spaces", args{"TO", "  <root@localhost>"}, "TO:  <root@localhost>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getRaw(tt.args.key, tt.args.value); string(got) != tt.want {
				t.Errorf("getRaw() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_headerField_Deleted(t *testing.T) {
	f := &Field{
		CanonicalKey: "To",
		Raw:          []byte("To: <root@localhost>"),
		deleted:      true,
	}
	if got := f.Deleted(); got != true {
		t.Errorf("%v.Deleted() = false, want true", f)
	}
	f = &Field{
		CanonicalKey: "To",
		Raw:          []byte("To:"),
		deleted:      true,
	}
	if got := f.Deleted(); got != true {
		t.Errorf("%v.Deleted() = false, want true", f)
	}
	f = &Field{
		CanonicalKey: "To",
		Raw:          []byte("To: <root@localhost>"),
		deleted:      false,
	}
	if got := f.Deleted(); got != false {
		t.Errorf("%v.Deleted() = true, want false", f)
	}
	f = &Field{
		CanonicalKey: "To",
		Raw:          []byte("To:"),
		deleted:      false,
	}
	if got := f.Deleted(); got != false {
		t.Errorf("%v.Deleted() = true, want false", f)
	}

	hdrs := testHeader()
	fields := hdrs.Fields()
	for fields.Next() {
		if fields.CanonicalKey() == "Subject" {
			fields.Del()
			break
		}
	}
	found := false
	fields = hdrs.Fields()
	for fields.Next() {
		if fields.CanonicalKey() == "Subject" {
			if !fields.IsDeleted() {
				t.Errorf("Subject should be deleted")
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Subject not found")
	}
	hdrs = testHeader()
	fields = hdrs.Fields()
	for fields.Next() {
		if fields.CanonicalKey() == "Subject" {
			fields.Del()
			break
		}
	}
	hdrs.SetText("Subject", "undeleted")
	expected := "From: <root@localhost>\r\nTo:  <root@localhost>, <nobody@localhost>\r\nsubject: undeleted\r\nDATE:\tWed, 01 Mar 2023 15:47:33 +0100\r\n\r\n"
	if got := outputFields(hdrs.fields); got != expected {
		t.Errorf("got %q, expect %q", got, expected)
	}

}

func Test_headerField_key(t *testing.T) {
	type fields struct {
		canonicalKey string
		raw          string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"same as canonical", fields{"To", "To: <root@localhost>"}, "To"},
		{"different", fields{"To", "TO: <root@localhost>"}, "TO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Field{
				CanonicalKey: tt.fields.canonicalKey,
				Raw:          []byte(tt.fields.raw),
			}
			if got := f.Key(); got != tt.want {
				t.Errorf("key() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_headerField_value(t *testing.T) {
	type fields struct {
		canonicalKey string
		raw          string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"value", fields{"To", "To: <root@localhost>"}, " <root@localhost>"},
		{"empty", fields{"To", "To:"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Field{
				CanonicalKey: tt.fields.canonicalKey,
				Raw:          []byte(tt.fields.raw),
			}
			if got := f.Value(); got != tt.want {
				t.Errorf("value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		want    *Header
		wantErr bool
	}{
		{"empty", []byte("  totally bogus 💣 header data "), nil, true},
		{"works", []byte("Subject: test\r\n"), &Header{fields: []*Field{{Index: 0, CanonicalKey: "Subject", Raw: []byte("Subject: test")}}}, false},
		{"brokenEncodingOk", []byte("Content-Type: text/plain; charset=bogus\r\nSubject: test\r\n"), &Header{fields: []*Field{{Index: 0, CanonicalKey: "Content-Type", Raw: []byte("Content-Type: text/plain; charset=bogus")}, {Index: 1, CanonicalKey: "Subject", Raw: []byte("Subject: test")}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_Len(t *testing.T) {
	tests := []struct {
		name   string
		fields []*Field
		want   int
	}{
		{"nil", nil, 0},
		{"empty", []*Field{}, 0},
		{"one", []*Field{{0, "Test", []byte("Test:"), false}}, 1},
		{"one-del", []*Field{{0, "Test", []byte("Test:"), true}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
				helper: newHelper(),
			}
			if got := h.Len(); got != tt.want {
				t.Errorf("Len() = %v, want %v", got, tt.want)
			}
		})
	}
}
