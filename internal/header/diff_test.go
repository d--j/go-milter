package header

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func outputDiff(diff []fieldDiff) string {
	s := strings.Builder{}
	for i, d := range diff {
		s.WriteString(fmt.Sprintf("%02d %02d ", i, d.index))
		switch d.kind {
		case KindEqual:
			s.WriteString("equal  ")
		case KindInsert:
			s.WriteString("insert ")
		case KindChange:
			s.WriteString("change ")
		}
		s.WriteString(fmt.Sprintf("(c:%s raw:%q idx:%d)\n", d.field.CanonicalKey, d.field.Raw, d.field.Index))
	}
	return s.String()
}

func Test_diffFields(t *testing.T) {
	orig := testHeader()
	addOne := testHeader()
	addOne.Add("X-Test", "1")
	addOneInFront := testHeader()
	fields := addOneInFront.Fields()
	fields.Next()
	fields.InsertBefore("X-Test", "1")
	equals := []fieldDiff{
		{KindEqual, orig.fields[0], 0},
		{KindEqual, orig.fields[1], 1},
		{KindEqual, orig.fields[2], 2},
		{KindEqual, orig.fields[3], 3},
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
	xTest := Field{-1, "X-Test", []byte("X-Test: 1"), false}
	subjectChanged := Field{2, "Subject", []byte("subject: changed"), false}
	dateDel := Field{3, "Date", []byte("DATE:"), false}

	type args struct {
		orig    []*Field
		changed []*Field
	}
	tests := []struct {
		name      string
		args      args
		wantDiffs []fieldDiff
	}{
		{"equal", args{orig.fields, orig.fields}, equals},
		{"add-one", args{orig.fields, addOne.fields}, append(equals, fieldDiff{KindInsert, &xTest, 3})},
		{"add-one-in-front", args{orig.fields, addOneInFront.fields}, append([]fieldDiff{{KindInsert, &xTest, -1}}, equals...)},
		{"complex", args{orig.fields, complexChanges.fields}, []fieldDiff{
			{KindInsert, &xTest, -1},
			equals[0],
			{KindInsert, &xTest, 0},
			{KindInsert, &xTest, 0},
			equals[1],
			{KindInsert, &xTest, 1},
			{KindInsert, &xTest, 1},
			{KindChange, &subjectChanged, 2},
			{KindInsert, &xTest, 2},
			{KindInsert, &xTest, 2},
			{KindChange, &dateDel, 3},
			{KindInsert, &xTest, 3},
			{KindInsert, &xTest, 3},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotDiffs := diffFields(tt.args.orig, tt.args.changed, -1); !reflect.DeepEqual(gotDiffs, tt.wantDiffs) {
				t.Errorf("diffFields() = %s, want %s", outputDiff(gotDiffs), outputDiff(tt.wantDiffs))
			}
		})
	}
}

func TestDiff(t *testing.T) {
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
		wantChangeInsertOps []Op
		wantAddOps          []Op
	}{
		{"equal", args{orig, orig}, nil, nil},
		{"add-one", args{orig, addOne}, nil, []Op{{Index: 5, Name: "X-Test", Value: " 1"}}},
		{"add-one-in-front", args{orig, addOneInFront}, []Op{{Kind: KindInsert, Index: 1, Name: "X-Test", Value: " 1"}}, nil},
		{"complex", args{orig, complexChanges}, []Op{
			{Kind: KindInsert, Index: 1, Name: "X-Test", Value: " 1"},
			{Kind: KindInsert, Index: 2, Name: "X-Test", Value: " 1"},
			{Kind: KindInsert, Index: 2, Name: "X-Test", Value: " 1"},
			{Kind: KindInsert, Index: 3, Name: "X-Test", Value: " 1"},
			{Kind: KindInsert, Index: 3, Name: "X-Test", Value: " 1"},
			{Kind: KindChange, Index: 1, Name: "subject", Value: " changed"},
			{Kind: KindInsert, Index: 4, Name: "X-Test", Value: " 1"},
			{Kind: KindInsert, Index: 4, Name: "X-Test", Value: " 1"},
			{Kind: KindChange, Index: 1, Name: "DATE", Value: ""},
		}, []Op{
			{Index: 5, Name: "X-Test", Value: " 1"},
			{Index: 5, Name: "X-Test", Value: " 1"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotChangeInsertOps, gotAddOps := Diff(tt.args.orig, tt.args.changed)
			if !reflect.DeepEqual(gotChangeInsertOps, tt.wantChangeInsertOps) {
				t.Errorf("Diff() gotChangeInsertOps = %+v, want %+v", gotChangeInsertOps, tt.wantChangeInsertOps)
			}
			if !reflect.DeepEqual(gotAddOps, tt.wantAddOps) {
				t.Errorf("Diff() gotAddOps = %+v, want %+v", gotAddOps, tt.wantAddOps)
			}
		})
	}
}

func TestRecreate(t *testing.T) {
	orig := testHeader()
	addOne := testHeader()
	addOne.Add("X-Test", "1")
	delFirst := testHeader()
	delFirstF := delFirst.Fields()
	delFirstF.Next()
	delFirstF.Del()
	type args struct {
		orig    *Header
		changed *Header
	}
	tests := []struct {
		name                string
		args                args
		wantChangeInsertOps []Op
		wantAddOps          []Op
	}{
		{"equal", args{orig, orig}, []Op{
			{Kind: KindChange, Index: 1, Name: "From", Value: ""},
			{Kind: KindChange, Index: 1, Name: "To", Value: ""},
			{Kind: KindChange, Index: 1, Name: "subject", Value: ""},
			{Kind: KindChange, Index: 1, Name: "DATE", Value: ""},
		}, []Op{
			{Index: 0, Name: "From", Value: " <root@localhost>"},
			{Index: 1, Name: "To", Value: "  <root@localhost>, <nobody@localhost>"},
			{Index: 2, Name: "subject", Value: " =?UTF-8?Q?=F0=9F=9F=A2?="},
			{Index: 3, Name: "DATE", Value: "\tWed, 01 Mar 2023 15:47:33 +0100"},
		}},
		{"add-one", args{orig, addOne}, []Op{
			{Kind: KindChange, Index: 1, Name: "From", Value: ""},
			{Kind: KindChange, Index: 1, Name: "To", Value: ""},
			{Kind: KindChange, Index: 1, Name: "subject", Value: ""},
			{Kind: KindChange, Index: 1, Name: "DATE", Value: ""},
		}, []Op{
			{Index: 0, Name: "From", Value: " <root@localhost>"},
			{Index: 1, Name: "To", Value: "  <root@localhost>, <nobody@localhost>"},
			{Index: 2, Name: "subject", Value: " =?UTF-8?Q?=F0=9F=9F=A2?="},
			{Index: 3, Name: "DATE", Value: "\tWed, 01 Mar 2023 15:47:33 +0100"},
			{Index: 4, Name: "X-Test", Value: " 1"},
		}},
		{"del-first", args{orig, delFirst}, []Op{
			{Kind: KindChange, Index: 1, Name: "From", Value: ""},
			{Kind: KindChange, Index: 1, Name: "To", Value: ""},
			{Kind: KindChange, Index: 1, Name: "subject", Value: ""},
			{Kind: KindChange, Index: 1, Name: "DATE", Value: ""},
		}, []Op{
			{Index: 0, Name: "To", Value: "  <root@localhost>, <nobody@localhost>"},
			{Index: 1, Name: "subject", Value: " =?UTF-8?Q?=F0=9F=9F=A2?="},
			{Index: 2, Name: "DATE", Value: "\tWed, 01 Mar 2023 15:47:33 +0100"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotChangeInsertOps, gotAddOps := Recreate(tt.args.orig, tt.args.changed)
			if !reflect.DeepEqual(gotChangeInsertOps, tt.wantChangeInsertOps) {
				t.Errorf("Recreate() gotChangeInsertOps = %+v, want %+v", gotChangeInsertOps, tt.wantChangeInsertOps)
			}
			if !reflect.DeepEqual(gotAddOps, tt.wantAddOps) {
				t.Errorf("Recreate() gotAddOps = %+v, want %+v", gotAddOps, tt.wantAddOps)
			}
		})
	}
}

func TestDiffOrRecreate(t *testing.T) {
	orig := testHeader()
	type args struct {
		recreate bool
		orig     *Header
		changed  *Header
	}
	tests := []struct {
		name                string
		args                args
		wantChangeInsertOps []Op
		wantAddOps          []Op
	}{
		{"diff", args{false, orig, orig}, nil, nil},
		{"recreate", args{true, orig, orig}, []Op{
			{Kind: KindChange, Index: 1, Name: "From", Value: ""},
			{Kind: KindChange, Index: 1, Name: "To", Value: ""},
			{Kind: KindChange, Index: 1, Name: "subject", Value: ""},
			{Kind: KindChange, Index: 1, Name: "DATE", Value: ""},
		}, []Op{
			{Index: 0, Name: "From", Value: " <root@localhost>"},
			{Index: 1, Name: "To", Value: "  <root@localhost>, <nobody@localhost>"},
			{Index: 2, Name: "subject", Value: " =?UTF-8?Q?=F0=9F=9F=A2?="},
			{Index: 3, Name: "DATE", Value: "\tWed, 01 Mar 2023 15:47:33 +0100"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotChangeInsertOps, gotAddOps := DiffOrRecreate(tt.args.recreate, tt.args.orig, tt.args.changed)
			if !reflect.DeepEqual(gotChangeInsertOps, tt.wantChangeInsertOps) {
				t.Errorf("DiffOrRecreate() gotChangeInsertOps = %+v, want %+v", gotChangeInsertOps, tt.wantChangeInsertOps)
			}
			if !reflect.DeepEqual(gotAddOps, tt.wantAddOps) {
				t.Errorf("DiffOrRecreate() gotAddOps = %+v, want %+v", gotAddOps, tt.wantAddOps)
			}
		})
	}
}
