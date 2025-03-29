package testtrx

import "testing"

func TestDiffModifications(t *testing.T) {
	delOne := Modification{Kind: DelRcptTo, Addr: "one@example.com"}
	delTwo := Modification{Kind: DelRcptTo, Addr: "two@example.com"}
	insertHeader := Modification{Kind: InsertHeader, Name: "X-Header-1", Value: "Value1"}
	changeHeader := Modification{Kind: ChangeHeader, Name: "X-Header-2", Value: "Value2"}
	replaceBody := Modification{Kind: ReplaceBody, Body: []byte("Body")}
	type args struct {
		expected []Modification
		got      []Modification
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"Empty", args{[]Modification{}, []Modification{}}, ""},
		{"Equal", args{[]Modification{insertHeader, delOne, changeHeader, replaceBody}, []Modification{replaceBody, insertHeader, delOne, changeHeader}}, ""},
		{"DelRcptToOrderDoesNotMatter", args{[]Modification{delOne, delTwo}, []Modification{delTwo, delOne}}, ""},
		{"Add", args{[]Modification{delOne}, []Modification{delTwo, delOne}}, "--- expected.txt\n+++ got.txt\n@@ -1,1 +1,2 @@\n DelRcptTo \"one@example.com\"\n+DelRcptTo \"two@example.com\"\n"},
		{"Remove", args{[]Modification{delTwo, delOne}, []Modification{delOne}}, "--- expected.txt\n+++ got.txt\n@@ -1,2 +1,1 @@\n DelRcptTo \"one@example.com\"\n-DelRcptTo \"two@example.com\"\n"},
		{"Remove2", args{[]Modification{insertHeader, delOne}, []Modification{delOne}}, "--- expected.txt\n+++ got.txt\n@@ -1,2 +1,1 @@\n DelRcptTo \"one@example.com\"\n-InsertHeader 0 \"X-Header-1\" \"Value1\"\n"},
		{"NoPrefix", args{[]Modification{insertHeader}, []Modification{delOne}}, "--- expected.txt\n+++ got.txt\n@@ -1,1 +1,1 @@\n-InsertHeader 0 \"X-Header-1\" \"Value1\"\n+DelRcptTo \"one@example.com\"\n"},
		{"MiddleDelete", args{[]Modification{delOne, delTwo, insertHeader}, []Modification{delOne, insertHeader}}, "--- expected.txt\n+++ got.txt\n@@ -1,3 +1,2 @@\n DelRcptTo \"one@example.com\"\n-DelRcptTo \"two@example.com\"\n+InsertHeader 0 \"X-Header-1\" \"Value1\"\n-InsertHeader 0 \"X-Header-1\" \"Value1\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DiffModifications(tt.args.expected, tt.args.got); got != tt.want {
				t.Errorf("DiffModifications() = %v, want %v", got, tt.want)
			}
		})
	}
}
