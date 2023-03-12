package mailfilter

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-message/mail"
)

func testHeader() *Header {
	return &Header{fields: []*headerField{
		{
			index:        0,
			canonicalKey: "From",
			raw:          "From: <root@localhost>",
		},
		{
			index:        1,
			canonicalKey: "To",
			raw:          "To:  <root@localhost>, <nobody@localhost>",
		},
		{
			index:        2,
			canonicalKey: "Subject",
			raw:          "subject: =?UTF-8?Q?=F0=9F=9F=A2?=", // 🟢
		},
		{
			index:        3,
			canonicalKey: "Date",
			raw:          "DATE:\tWed, 01 Mar 2023 15:47:33 +0100",
		},
	}}
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
			f := &HeaderFields{
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
		want   *headerField
	}{
		{"First", fields{0, testHeader()}, &headerField{0, "From", "From:"}},
		{"Third", fields{2, testHeader()}, &headerField{2, "Subject", "subject:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &HeaderFields{
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

func TestHeaderFields_Get(t *testing.T) {
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
			f := &HeaderFields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			if got := f.Get(); got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
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
			f := &HeaderFields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			got, err := f.GetAddressList()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAddressList() got = %v, want %v", got, tt.want)
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
			f := &HeaderFields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			got, err := f.GetText()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetText() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func outputFields(fields []*headerField) string {
	h := Header{fields: fields}
	bytes, _ := io.ReadAll(h.Reader())
	return string(bytes)
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
	expectOne := []*headerField{{-1, "Test", "Test: one"}}
	addTwo := []args{{"Test", "one"}, {"Test", "two"}}
	expectTwo := []*headerField{{-1, "Test", "Test: one"}, {-1, "Test", "Test: two"}}
	tests := []struct {
		name     string
		fields   fields
		args     []args
		want     []*headerField
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
			f := &HeaderFields{
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
	expectOne := []*headerField{{-1, "Test", "Test: one"}}
	addTwo := []args{{"Test", "one"}, {"Test", "two"}}
	expectTwo := []*headerField{{-1, "Test", "Test: one"}, {-1, "Test", "Test: two"}}
	tests := []struct {
		name     string
		fields   fields
		args     []args
		want     []*headerField
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
			f := &HeaderFields{
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
			f := &HeaderFields{
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
			f := &HeaderFields{
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
			f := &HeaderFields{
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
		want   []*headerField
	}{
		{"works", fields{0, testHeader()}, args{"new", "header"}, append([]*headerField{{0, "New", "new: header"}}, testHeader().fields[1:]...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &HeaderFields{
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
		want   *headerField
	}{
		{"First", fields{0, testHeader()}, args{"set"}, &headerField{0, "From", "From: set"}},
		{"Third", fields{2, testHeader()}, args{"\tset"}, &headerField{2, "Subject", "subject:\tset"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &HeaderFields{
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
		want   *headerField
	}{
		{"One", fields{0, testHeader()}, args{[]*mail.Address{&nobody}}, &headerField{0, "From", "From: <nobody@localhost>"}},
		{"Two", fields{1, testHeader()}, args{[]*mail.Address{&nobody, &root}}, &headerField{1, "To", "To: <nobody@localhost>, <root@localhost>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &HeaderFields{
				cursor: tt.fields.cursor,
				h:      tt.fields.h,
				helper: newHelper(),
			}
			f.SetAddressList(tt.args.value)
			got := f.h.fields[f.index()]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetAddressList() = %v, want %v", got, tt.want)
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
		want   *headerField
	}{
		{"Set", fields{0, testHeader()}, args{"set"}, &headerField{0, "From", "From: set"}},
		{"UTF-8", fields{2, testHeader()}, args{"🔴"}, &headerField{2, "Subject", "subject: =?utf-8?q?=F0=9F=94=B4?="}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &HeaderFields{
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
		fields []*headerField
		args   args
		want   []*headerField
	}{
		{"works", testHeader().fields, args{"key", "value"}, append(testHeader().fields, &headerField{-1, "Key", "key: value"})},
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
	brokenDate.fields[3].raw = "Date: broken"
	tests := []struct {
		name    string
		fields  []*headerField
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
			fields.InsertBefore("X-From1", fields.Get())
			fields.InsertBefore("X-From2", fields.Get())
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
}

func TestHeader_Get(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name   string
		fields []*headerField
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
			if got := h.Get(tt.args.key); got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
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
		fields  []*headerField
		args    args
		want    []*mail.Address
		wantErr bool
	}{
		{"From", testHeader().fields, args{"fRoM"}, []*mail.Address{&root}, false},
		{"To", testHeader().fields, args{"tO"}, []*mail.Address{&root, &nobody}, false},
		{"Subject", testHeader().fields, args{"SUBJECT"}, nil, true},
		{"Date", testHeader().fields, args{"Date"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			got, err := h.GetAddressList(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAddressList() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_GetText(t *testing.T) {
	brokenSubject := testHeader()
	brokenSubject.fields[2].raw = "Subject: =?e-404?Q?=F0=9F=9F=A2?="
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  []*headerField
		args    args
		want    string
		wantErr bool
	}{
		{"works", testHeader().fields, args{"subJeCt"}, " 🟢", false},
		{"broken", brokenSubject.fields, args{"subJeCt"}, " =?e-404?Q?=F0=9F=9F=A2?=", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			got, err := h.GetText(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetText() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeader_Reader(t *testing.T) {
	tests := []struct {
		name   string
		fields []*headerField
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
		fields []*headerField
		args   args
		want   []*headerField
	}{
		{"found", testHeader().fields, args{"suBJect", "value"}, append(testHeader().fields[:2], append([]*headerField{{2, "Subject", "subject: value"}}, testHeader().fields[3:]...)...)},
		{"not-found", testHeader().fields, args{"x-spam", "value"}, append(testHeader().fields, &headerField{-1, "X-Spam", "x-spam: value"})},
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
		fields []*headerField
		args   args
		want   []*headerField
	}{
		{"works", testHeader().fields, args{"x-to", []*mail.Address{&root}}, append(testHeader().fields, &headerField{-1, "X-To", "x-to: <root@localhost>"})},
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
		fields []*headerField
		args   args
		want   []*headerField
	}{
		{"works", testHeader().fields, args{time.Date(1980, time.January, 1, 12, 0, 0, 0, time.UTC)}, append(testHeader().fields[:3], &headerField{3, "Date", "DATE: Tue, 01 Jan 1980 12:00:00 +0000"})},
		{"zero-ok", testHeader().fields, args{time.Time{}}, append(testHeader().fields[:3], &headerField{3, "Date", "DATE:"})},
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
		fields []*headerField
		args   args
		want   []*headerField
	}{
		{"works", testHeader().fields, args{"set"}, append(testHeader().fields[:2], &headerField{2, "Subject", "subject: set"}, testHeader().fields[3])},
		{"zero-ok", testHeader().fields, args{""}, append(testHeader().fields[:2], &headerField{2, "Subject", "subject:"}, testHeader().fields[3])},
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
		fields []*headerField
		args   args
		want   []*headerField
	}{
		{"works", testHeader().fields, args{"SubJect", "set"}, append(testHeader().fields[:2], &headerField{2, "Subject", "subject: set"}, testHeader().fields[3])},
		{"zero-ok", testHeader().fields, args{"Subject", ""}, append(testHeader().fields[:2], &headerField{2, "Subject", "subject:"}, testHeader().fields[3])},
		{"add", testHeader().fields, args{"x-red", "🔴"}, append(testHeader().fields, &headerField{-1, "X-Red", "x-red: =?utf-8?q?=F0=9F=94=B4?="})},
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
	brokenSubject.fields[2].raw = "Subject: =?e-404?Q?=F0=9F=9F=A2?="
	tests := []struct {
		name    string
		fields  []*headerField
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
		fields []*headerField
		args   args
		want   []*headerField
	}{
		{"works", nil, args{key: "TEST", raw: "TEST: value"}, []*headerField{{canonicalKey: "Test", raw: "TEST: value"}}},
		{"empty-is-ok", nil, args{key: "TEST", raw: "TEST:"}, []*headerField{{canonicalKey: "Test", raw: "TEST:"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				fields: tt.fields,
			}
			h.addRaw(tt.args.key, tt.args.raw)
		})
	}
}

func TestHeader_copy(t *testing.T) {
	h := Header{fields: []*headerField{{0, "Test", "Test:"}}}
	h2 := h.copy()
	h.fields[0].canonicalKey = "Changed"
	if len(h2.fields) != len(h.fields) {
		t.Fatal("did not copy fields")
	}
	if h.fields[0].canonicalKey == h2.fields[0].canonicalKey {
		t.Fatal("did not copy deep copy fields")
	}
}

func outputDiff(diff []headerFieldDiff) string {
	s := strings.Builder{}
	for i, d := range diff {
		s.WriteString(fmt.Sprintf("%02d %02d ", i, d.index))
		switch d.kind {
		case kindEqual:
			s.WriteString("equal  ")
		case kindInsert:
			s.WriteString("insert ")
		case kindChange:
			s.WriteString("change ")
		}
		s.WriteString(fmt.Sprintf("(c:%s raw:%q idx:%d)\n", d.field.canonicalKey, d.field.raw, d.field.index))
	}
	return s.String()
}

func Test_diffHeaderFields(t *testing.T) {
	orig := testHeader()
	addOne := testHeader()
	addOne.Add("X-Test", "1")
	addOneInFront := testHeader()
	fields := addOneInFront.Fields()
	fields.Next()
	fields.InsertBefore("X-Test", "1")
	equals := []headerFieldDiff{
		{kindEqual, orig.fields[0], 0},
		{kindEqual, orig.fields[1], 1},
		{kindEqual, orig.fields[2], 2},
		{kindEqual, orig.fields[3], 3},
	}
	complexChanges := testHeader()
	fields = complexChanges.Fields()
	for fields.Next() {
		fields.InsertBefore("X-Test", "1")
		fields.InsertAfter("X-Test", "1")
		if fields.CanonicalKey() == "Subject" {
			fields.Set("changed")
		}
		if fields.CanonicalKey() == "Date" {
			fields.Replace("X-Test", "1")
		}
	}
	xTest := headerField{-1, "X-Test", "X-Test: 1"}
	subjectChanged := headerField{2, "Subject", "subject: changed"}
	dateDel := headerField{3, "Date", "DATE:"}

	type args struct {
		orig    []*headerField
		changed []*headerField
	}
	tests := []struct {
		name      string
		args      args
		wantDiffs []headerFieldDiff
	}{
		{"equal", args{orig.fields, orig.fields}, equals},
		{"add-one", args{orig.fields, addOne.fields}, append(equals, headerFieldDiff{kindInsert, &xTest, 3})},
		{"add-one-in-front", args{orig.fields, addOneInFront.fields}, append([]headerFieldDiff{{kindInsert, &xTest, -1}}, equals...)},
		{"complex", args{orig.fields, complexChanges.fields}, []headerFieldDiff{
			{kindInsert, &xTest, -1},
			equals[0],
			{kindInsert, &xTest, 0},
			{kindInsert, &xTest, 0},
			equals[1],
			{kindInsert, &xTest, 1},
			{kindInsert, &xTest, 1},
			{kindChange, &subjectChanged, 2},
			{kindInsert, &xTest, 2},
			{kindInsert, &xTest, 2},
			{kindChange, &dateDel, 3},
			{kindInsert, &xTest, 3},
			{kindInsert, &xTest, 3},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotDiffs := diffHeaderFields(tt.args.orig, tt.args.changed, -1); !reflect.DeepEqual(gotDiffs, tt.wantDiffs) {
				t.Errorf("diffHeaderFields() = %s, want %s", outputDiff(gotDiffs), outputDiff(tt.wantDiffs))
			}
		})
	}
}

func Test_calculateHeaderModifications(t *testing.T) {
	orig := testHeader()
	addOne := testHeader()
	addOne.Add("X-Test", "1")
	addOneInFront := testHeader()
	fields := addOneInFront.Fields()
	fields.Next()
	fields.InsertBefore("X-Test", "1")
	complexChanges := testHeader()
	fields = complexChanges.Fields()
	for fields.Next() {
		fields.InsertBefore("X-Test", "1")
		fields.InsertAfter("X-Test", "1")
		if fields.CanonicalKey() == "Subject" {
			fields.Set("changed")
		}
		if fields.CanonicalKey() == "Date" {
			fields.Replace("X-Test", "1")
		}
	}
	type args struct {
		orig    *Header
		changed *Header
	}
	tests := []struct {
		name                string
		args                args
		wantChangeInsertOps []headerOp
		wantAddOps          []headerOp
	}{
		{"equal", args{orig, orig}, nil, nil},
		{"add-one", args{orig, addOne}, nil, []headerOp{{Index: 5, Name: "X-Test", Value: " 1"}}},
		{"add-one-in-front", args{orig, addOneInFront}, []headerOp{{Kind: kindInsert, Index: 0, Name: "X-Test", Value: " 1"}}, nil},
		{"complex", args{orig, complexChanges}, []headerOp{
			{Kind: kindInsert, Index: 0, Name: "X-Test", Value: " 1"},
			{Kind: kindInsert, Index: 2, Name: "X-Test", Value: " 1"},
			{Kind: kindInsert, Index: 2, Name: "X-Test", Value: " 1"},
			{Kind: kindInsert, Index: 3, Name: "X-Test", Value: " 1"},
			{Kind: kindInsert, Index: 3, Name: "X-Test", Value: " 1"},
			{Kind: kindChange, Index: 1, Name: "subject", Value: " changed"},
			{Kind: kindInsert, Index: 4, Name: "X-Test", Value: " 1"},
			{Kind: kindInsert, Index: 4, Name: "X-Test", Value: " 1"},
			{Kind: kindChange, Index: 1, Name: "DATE", Value: ""},
		}, []headerOp{
			{Index: 5, Name: "X-Test", Value: " 1"},
			{Index: 5, Name: "X-Test", Value: " 1"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotChangeInsertOps, gotAddOps := calculateHeaderModifications(tt.args.orig, tt.args.changed)
			if !reflect.DeepEqual(gotChangeInsertOps, tt.wantChangeInsertOps) {
				t.Errorf("calculateHeaderModifications() gotChangeInsertOps = %+v, want %+v", gotChangeInsertOps, tt.wantChangeInsertOps)
			}
			if !reflect.DeepEqual(gotAddOps, tt.wantAddOps) {
				t.Errorf("calculateHeaderModifications() gotAddOps = %+v, want %+v", gotAddOps, tt.wantAddOps)
			}
		})
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
			if got := getRaw(tt.args.key, tt.args.value); got != tt.want {
				t.Errorf("getRaw() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_headerField_deleted(t *testing.T) {
	type fields struct {
		canonicalKey string
		raw          string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{"deleted", fields{"To", "To:"}, true},
		{"not deleted", fields{"To", "To: <root@localhost>"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &headerField{
				canonicalKey: tt.fields.canonicalKey,
				raw:          tt.fields.raw,
			}
			if got := f.deleted(); got != tt.want {
				t.Errorf("deleted() = %v, want %v", got, tt.want)
			}
		})
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
			f := &headerField{
				canonicalKey: tt.fields.canonicalKey,
				raw:          tt.fields.raw,
			}
			if got := f.key(); got != tt.want {
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
			f := &headerField{
				canonicalKey: tt.fields.canonicalKey,
				raw:          tt.fields.raw,
			}
			if got := f.value(); got != tt.want {
				t.Errorf("value() = %v, want %v", got, tt.want)
			}
		})
	}
}
